package chat

import (
	"context"
	"fmt"
	"time"
	. "xiaozhi-esp32-server-golang/internal/data/client"
	"xiaozhi-esp32-server-golang/internal/domain/audio"
	log "xiaozhi-esp32-server-golang/logger"
)

type ASRManagerOption func(*ASRManager)

type ASRManager struct {
	clientState     *ClientState
	serverTransport *ServerTransport
}

func NewASRManager(clientState *ClientState, serverTransport *ServerTransport, opts ...ASRManagerOption) *ASRManager {
	asr := &ASRManager{
		clientState:     clientState,
		serverTransport: serverTransport,
	}
	for _, opt := range opts {
		opt(asr)
	}
	return asr
}

// ProcessVadAudio 启动VAD音频处理
func (a *ASRManager) ProcessVadAudio(ctx context.Context) {
	state := a.clientState
	go func() {
		audioFormat := state.InputAudioFormat
		audioProcesser, err := audio.GetAudioProcesser(audioFormat.SampleRate, audioFormat.Channels, audioFormat.FrameDuration)
		if err != nil {
			log.Errorf("获取解码器失败: %v", err)
			return
		}
		frameSize := state.AsrAudioBuffer.PcmFrameSize
		pcmFrame := make([]float32, frameSize)

		vadNeedGetCount := 1
		if state.DeviceConfig.Vad.Provider == "silero_vad" {
			vadNeedGetCount = 60 / audioFormat.FrameDuration
		}

		for {
			select {
			case opusFrame, ok := <-state.OpusAudioBuffer:
				if !ok {
					log.Debugf("processAsrAudio 音频通道已关闭")
					return
				}

				var skipVad bool
				var haveVoice bool
				clientHaveVoice := state.GetClientHaveVoice()
				if state.Asr.AutoEnd || state.ListenMode == "manual" {
					skipVad = true         //跳过vad
					clientHaveVoice = true //之前有声音
					haveVoice = true       //本次有声音
				}

				if state.GetClientVoiceStop() { //已停止 说话 则不接收音频数据
					//log.Infof("客户端停止说话, 跳过音频数据")
					continue
				}

				n, err := audioProcesser.DecoderFloat32(opusFrame, pcmFrame)
				if err != nil {
					log.Errorf("解码失败: %v", err)
					continue
				}

				var vadPcmData []float32
				pcmData := pcmFrame[:n]
				if !skipVad {
					//如果已经检测到语音, 则不进行vad检测, 直接将pcmData传给asr
					if state.VadProvider == nil {
						// 初始化vad
						err = state.Vad.Init(state.DeviceConfig.Vad.Provider, state.DeviceConfig.Vad.Config)
						if err != nil {
							log.Errorf("初始化vad失败: %v", err)
							continue
						}
					}
					//decode opus to pcm
					state.AsrAudioBuffer.AddAsrAudioData(pcmData)

					if state.AsrAudioBuffer.GetAsrDataSize() >= vadNeedGetCount*state.AsrAudioBuffer.PcmFrameSize {
						//如果要进行vad, 至少要取60ms的音频数据
						vadPcmData = state.AsrAudioBuffer.GetAsrData(vadNeedGetCount)
						state.VadProvider.Reset()
						haveVoice, err = state.VadProvider.IsVADExt(vadPcmData, audioFormat.SampleRate, frameSize)

						if err != nil {
							log.Errorf("processAsrAudio VAD检测失败: %v", err)
							//删除
							continue
						}
						//首次触发识别到语音时,为了语音数据完整性 将vadPcmData赋值给pcmData, 之后的音频数据全部进入asr
						if haveVoice && !clientHaveVoice {
							//首次获取全部pcm数据送入asr
							pcmData = state.AsrAudioBuffer.GetAndClearAllData()
						}
					}
					//log.Debugf("isVad, pcmData len: %d, vadPcmData len: %d, haveVoice: %v", len(pcmData), len(vadPcmData), haveVoice)
				}

				if haveVoice {
					log.Infof("检测到语音, len: %d", len(pcmData))
					state.SetClientHaveVoice(true)
					state.SetClientHaveVoiceLastTime(time.Now().UnixMilli())
					state.Vad.ResetIdleDuration()
				} else {
					state.Vad.AddIdleDuration(int64(audioFormat.FrameDuration))
					idleDuration := state.Vad.GetIdleDuration()
					if idleDuration > state.GetMaxIdleDuration() {
						log.Infof("超出空闲时长: %dms, 断开连接", idleDuration)
						//断开连接
						a.serverTransport.Close()
						return
					}
					//如果之前没有语音, 本次也没有语音, 则从缓存中删除
					if !clientHaveVoice {
						//保留近10帧
						if state.AsrAudioBuffer.GetFrameCount() > vadNeedGetCount*3 {
							state.AsrAudioBuffer.RemoveAsrAudioData(1)
						}
						continue
					}
				}

				if clientHaveVoice {
					//vad识别成功, 往asr音频通道里发送数据
					if state.AsrAudioChannel != nil {
						state.AsrAudioChannel <- pcmData
					}
				}

				//已经有语音了, 但本次没有检测到语音, 则需要判断是否已经停止说话
				lastHaveVoiceTime := state.GetClientHaveVoiceLastTime()

				if clientHaveVoice && lastHaveVoiceTime > 0 && !haveVoice {
					idleDuration := state.Vad.GetIdleDuration()
					if state.IsSilence(idleDuration) { //从有声音到 静默的判断
						state.OnVoiceSilence()
						continue
					}
				}

			case <-ctx.Done():
				return
			}
		}
	}()
}

// restartAsrRecognition 重启ASR识别
func (a *ASRManager) RestartAsrRecognition(ctx context.Context) error {
	state := a.clientState
	log.Debugf("重启ASR识别开始")

	// 取消当前ASR上下文
	if state.Asr.Cancel != nil {
		state.Asr.Cancel()
	}

	state.VoiceStatus.Reset()
	state.AsrAudioBuffer.ClearAsrAudioData()

	// 等待一小段时间让资源清理
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// 重新创建ASR上下文和通道
	state.Asr.Ctx, state.Asr.Cancel = context.WithCancel(ctx)
	state.Asr.AsrAudioChannel = make(chan []float32, 100)

	// 重新启动流式识别
	asrResultChannel, err := state.AsrProvider.StreamingRecognize(state.Asr.Ctx, state.Asr.AsrAudioChannel)
	if err != nil {
		log.Errorf("重启ASR流式识别失败: %v", err)
		return fmt.Errorf("重启ASR流式识别失败: %v", err)
	}

	state.AsrResultChannel = asrResultChannel
	log.Debugf("重启ASR识别成功")
	return nil
}
