package common

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	log "xiaozhi-esp32-server-golang/logger"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
	"github.com/gopxl/beep"
	"github.com/gopxl/beep/mp3"
	"gopkg.in/hraban/opus.v2"
)

// min returns the smaller of x or y.
func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

// readCloserWrapper 为 bytes.Reader 提供 Close 方法以实现 ReadCloser 接口
type readCloserWrapper struct {
	*bytes.Reader
}

// Close 实现 io.Closer 接口
func (r *readCloserWrapper) Close() error {
	return nil
}

// newReadCloserWrapper 创建一个新的 ReadCloser 包装
func newReadCloserWrapper(data []byte) *readCloserWrapper {
	return &readCloserWrapper{bytes.NewReader(data)}
}

// WavToOpus 将WAV音频数据转换为标准Opus格式
// 返回Opus帧的切片集合，每个切片是一个Opus编码帧
func WavToOpus(wavData []byte, sampleRate int, channels int, bitRate int) ([][]byte, error) {
	// 创建WAV解码器
	wavReader := bytes.NewReader(wavData)
	wavDecoder := wav.NewDecoder(wavReader)
	if !wavDecoder.IsValidFile() {
		return nil, fmt.Errorf("无效的WAV文件")
	}

	// 读取WAV文件信息
	wavDecoder.ReadInfo()
	format := wavDecoder.Format()
	wavSampleRate := int(format.SampleRate)
	wavChannels := int(format.NumChannels)

	// 如果提供的参数与文件参数不一致，使用文件中的参数
	if sampleRate == 0 {
		sampleRate = wavSampleRate
	}
	if channels == 0 {
		channels = wavChannels
	}

	//打印wavDecoder信息
	fmt.Println("WAV格式:", format)

	enc, err := opus.NewEncoder(sampleRate, channels, opus.AppAudio)
	if err != nil {
		return nil, fmt.Errorf("创建Opus编码器失败: %v", err)
	}

	// 设置比特率
	if bitRate > 0 {
		if err := enc.SetBitrate(bitRate); err != nil {
			return nil, fmt.Errorf("设置比特率失败: %v", err)
		}
	}

	// 创建输出帧切片数组
	opusFrames := make([][]byte, 0)

	perFrameDuration := 20
	// PCM缓冲区 - Opus帧大小(60ms)
	frameSize := sampleRate * perFrameDuration / 1000
	pcmBuffer := make([]int16, frameSize*channels)
	opusBuffer := make([]byte, 1000) // 足够大的缓冲区存储编码后的数据

	// 读取音频缓冲区
	audioBuf := &audio.IntBuffer{Data: make([]int, frameSize*channels), Format: format}

	fmt.Println("开始转换...")
	for {
		// 读取WAV数据
		n, err := wavDecoder.PCMBuffer(audioBuf)
		if err == io.EOF || n == 0 {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("读取WAV数据失败: %v", err)
		}

		// 将int转换为int16
		for i := 0; i < len(audioBuf.Data); i++ {
			if i < len(pcmBuffer) {
				pcmBuffer[i] = int16(audioBuf.Data[i])
			}
		}

		// 编码为Opus格式
		n, err = enc.Encode(pcmBuffer, opusBuffer)
		if err != nil {
			return nil, fmt.Errorf("编码失败: %v", err)
		}

		// 将当前帧复制到新的切片中并添加到帧数组
		frameData := make([]byte, n)
		copy(frameData, opusBuffer[:n])
		opusFrames = append(opusFrames, frameData)
	}

	return opusFrames, nil
}

type AudioDecoder struct {
	streamer           beep.StreamSeekCloser
	format             beep.Format
	enc                *opus.Encoder
	pipeReader         io.ReadCloser
	perFrameDurationMs int
	AudioFormat        string

	outputOpusChan chan []byte     //opus一帧一帧的输出
	ctx            context.Context // 新增：上下文控制
}

// CreateMP3Decoder 创建一个通过 Done 通道控制的 MP3 解码器
// 为了兼容旧代码，保留此方法
func CreateAudioDecoder(ctx context.Context, pipeReader io.ReadCloser, outputOpusChan chan []byte, perFrameDurationMs int, AudioFormat string) (*AudioDecoder, error) {
	return &AudioDecoder{
		pipeReader:         pipeReader,
		outputOpusChan:     outputOpusChan,
		perFrameDurationMs: perFrameDurationMs,
		AudioFormat:        AudioFormat,
		ctx:                ctx,
	}, nil
}

func (d *AudioDecoder) WithFormat(format beep.Format) *AudioDecoder {
	d.format = format
	return d
}

func (d *AudioDecoder) Run(startTs int64) error {
	if d.AudioFormat == "wav" {
		d.RunWavDecoder(startTs, false)
	} else if d.AudioFormat == "pcm" {
		d.RunWavDecoder(startTs, true)
	} else if d.AudioFormat == "mp3" {
		return d.RunMp3Decoder(startTs)
	}
	return nil
}

func (d *AudioDecoder) RunWavDecoder(startTs int64, isRaw bool) error {
	defer close(d.outputOpusChan)

	var sampleRate int
	var channels int

	if !isRaw {
		// WAV文件头部固定为44字节
		headerSize := 44
		header := make([]byte, headerSize)
		_, err := io.ReadFull(d.pipeReader, header)
		if err != nil {
			return fmt.Errorf("读取WAV头部失败: %v", err)
		}

		// 从WAV头部获取基本参数
		// 采样率: 字节24-27
		sampleRate = int(uint32(header[24]) | uint32(header[25])<<8 | uint32(header[26])<<16 | uint32(header[27])<<24)
		// 通道数: 字节22-23
		channels = int(uint16(header[22]) | uint16(header[23])<<8)

		log.Debugf("WAV格式: %d Hz, %d 通道", sampleRate, channels)
	} else {
		// 对于原始PCM数据，使用format中的参数
		sampleRate = int(d.format.SampleRate)
		channels = d.format.NumChannels
		log.Debugf("原始PCM格式: %d Hz, %d 通道", sampleRate, channels)
	}

	// 始终使用单通道输出
	outputChannels := 1
	if channels > 1 {
		log.Debugf("将多声道音频转换为单声道输出")
	}

	enc, err := opus.NewEncoder(int(sampleRate), outputChannels, opus.AppAudio)
	if err != nil {
		return fmt.Errorf("创建Opus编码器失败: %v", err)
	}
	d.enc = enc

	//opus相关配置及缓冲区
	frameDurationMs := d.perFrameDurationMs              //每帧时长(ms)
	frameSize := sampleRate * frameDurationMs / 1000     //每帧采样点数
	pcmBuffer := make([]int16, frameSize*outputChannels) //PCM缓冲区
	opusBuffer := make([]byte, 1000)                     //Opus输出缓冲区

	// 用于读取原始PCM数据的缓冲区
	rawBuffer := make([]byte, frameSize*channels*2) // 16位采样=2字节
	currentFramePos := 0
	var firstFrame bool

	for {
		select {
		case <-d.ctx.Done():
			log.Debugf("wavDecoder context done, exit")
			return nil
		default:
			// 读取PCM数据
			n, err := d.pipeReader.Read(rawBuffer)
			if err == io.EOF {
				// 处理剩余不足一帧的数据
				if currentFramePos > 0 {
					paddedFrame := make([]int16, frameSize)
					copy(paddedFrame, pcmBuffer[:currentFramePos])

					// 编码最后一帧
					if n, err := d.enc.Encode(paddedFrame, opusBuffer); err == nil {
						frameData := make([]byte, n)
						copy(frameData, opusBuffer[:n])
						d.outputOpusChan <- frameData
					}
				}
				return nil
			}
			if err != nil {
				return fmt.Errorf("读取PCM数据失败: %v", err)
			}

			// 将字节数据转换为int16采样点
			samplesRead := n / (2 * channels) // 每个采样2字节,考虑通道数
			for i := 0; i < samplesRead; i++ {
				// 对于多通道,取平均值
				var sampleSum int32
				for ch := 0; ch < channels; ch++ {
					pos := i*channels*2 + ch*2
					sample := int16(uint16(rawBuffer[pos]) | uint16(rawBuffer[pos+1])<<8)
					sampleSum += int32(sample)
				}

				// 计算多通道平均值
				avgSample := int16(sampleSum / int32(channels))
				pcmBuffer[currentFramePos] = avgSample
				currentFramePos++

				// 如果缓冲区已满,进行编码
				if currentFramePos == frameSize {
					if n, err := d.enc.Encode(pcmBuffer, opusBuffer); err == nil {
						frameData := make([]byte, n)
						copy(frameData, opusBuffer[:n])

						if !firstFrame {
							firstFrame = true
							log.Infof("tts云端->首帧解码完成耗时: %d ms", time.Now().UnixMilli()-startTs)
						}

						d.outputOpusChan <- frameData
					}
					currentFramePos = 0
				}
			}
		}
	}
}

func (d *AudioDecoder) RunMp3Decoder(startTs int64) error {
	defer close(d.outputOpusChan)

	decoder, format, err := mp3.Decode(d.pipeReader)
	if err != nil {
		return fmt.Errorf("创建MP3解码器失败: %v", err)
	}
	log.Debugf("MP3格式: %d Hz, %d 通道", format.SampleRate, format.NumChannels)
	d.streamer = decoder
	d.format = format

	// 流式解码MP3
	defer func() {
		d.streamer.Close()
	}()

	// 获取MP3音频信息
	sampleRate := format.SampleRate
	channels := format.NumChannels

	// 始终使用单通道输出
	outputChannels := 1
	if channels > 1 {
		log.Debugf("将双声道音频转换为单声道输出")
	}

	enc, err := opus.NewEncoder(int(sampleRate), outputChannels, opus.AppAudio)
	if err != nil {
		return fmt.Errorf("创建Opus编码器失败: %v", err)
	}
	d.enc = enc

	//opus相关配置及缓冲区 创建缓冲区用于接收音频采样
	frameDurationMs := d.perFrameDurationMs               //60ms
	frameSize := int(sampleRate) * frameDurationMs / 1000 // 60ms帧大小
	// 临时PCM存储，将音频转换为PCM格式
	pcmBuffer := make([]int16, frameSize*outputChannels)

	//mp3读缓冲区
	mp3Buffer := make([][2]float64, 1024)

	//opus输出缓冲区
	opusBuffer := make([]byte, 1000)

	currentFramePos := 0 // 当前填充到pcmBuffer的位置
	var firstFrame bool
	for {
		select {
		case <-d.ctx.Done():
			log.Debugf("mp3Decoder context done, exit")
			return nil
		default:
			// 从MP3读取PCM数据
			n, ok := d.streamer.Stream(mp3Buffer)
			if !firstFrame {
				log.Infof("tts云端首帧耗时: %d ms", time.Now().UnixMilli()-startTs)
			}
			//fmt.Printf("for loop, n: %d\n", n)
			if !ok {
				// 处理剩余不足一帧的数据
				if currentFramePos > 0 {
					// 创建一个完整的帧缓冲区，用0填充剩余部分
					paddedFrame := make([]int16, len(pcmBuffer))
					copy(paddedFrame, pcmBuffer[:currentFramePos]) // 将有效数据复制到开头，剩余部分默认为0

					// 编码补齐后的完整帧
					n, err := enc.Encode(paddedFrame, opusBuffer)
					if err != nil {
						log.Errorf("编码剩余数据失败: %v\n", err)
						return fmt.Errorf("编码剩余数据失败: %v", err)
					} else {
						frameData := make([]byte, n)
						copy(frameData, opusBuffer[:n])

						select {
						case <-d.ctx.Done():
							log.Debugf("mp3Decoder context done, exit")
							return nil
						default:
							d.outputOpusChan <- frameData
						}
					}
				}
				return nil
			}

			if n == 0 {
				continue
			}
			// 将浮点音频数据转换为PCM格式(16位整数)
			for i := 0; i < n; i++ {
				// 先在浮点数阶段计算平均值，避免整数相加时溢出
				monoSampleFloat := (mp3Buffer[i][0] + mp3Buffer[i][1]) * 0.5

				// 进行音量限制，确保不超出范围
				if monoSampleFloat > 1.0 {
					monoSampleFloat = 1.0
				} else if monoSampleFloat < -1.0 {
					monoSampleFloat = -1.0
				}

				// 将浮点平均值转换为16位整数
				monoSample := int16(monoSampleFloat * 32767.0)
				pcmBuffer[currentFramePos] = monoSample
				currentFramePos++

				// 如果pcmBuffer已满一帧，则进行编码
				if currentFramePos == len(pcmBuffer) {
					opusLen, err := enc.Encode(pcmBuffer, opusBuffer)
					if err != nil {
						log.Errorf("编码失败: %v\n", err)
						continue
					}

					// 将当前帧复制到新的切片中并添加到帧数组
					frameData := make([]byte, opusLen)
					copy(frameData, opusBuffer[:opusLen])

					select {
					case <-d.ctx.Done():
						log.Debugf("mp3Decoder context done, exit")
						return nil
					default:
						if !firstFrame {
							firstFrame = true
							log.Infof("tts云端->首帧解码完成耗时: %d ms", time.Now().UnixMilli()-startTs)
						}

						d.outputOpusChan <- frameData
					}

					currentFramePos = 0 // 重置帧位置
				}
			}
		}
	}

	return nil
}
