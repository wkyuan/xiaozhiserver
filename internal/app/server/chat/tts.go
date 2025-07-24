package chat

import (
	"context"
	"fmt"
	"time"
	. "xiaozhi-esp32-server-golang/internal/data/client"
	llm_common "xiaozhi-esp32-server-golang/internal/domain/llm/common"
	"xiaozhi-esp32-server-golang/internal/util"
	log "xiaozhi-esp32-server-golang/logger"
)

type TTSQueueItem struct {
	ctx         context.Context
	llmResponse llm_common.LLMResponseStruct
	onStartFunc func()
	onEndFunc   func(err error)
}

// TTSManager 负责TTS相关的处理
// 可以根据需要扩展字段
// 目前无状态，但可后续扩展

type TTSManagerOption func(*TTSManager)

type TTSManager struct {
	clientState     *ClientState
	serverTransport *ServerTransport
	ttsQueue        *util.Queue[TTSQueueItem]
}

// NewTTSManager 只接受WithClientState
func NewTTSManager(clientState *ClientState, serverTransport *ServerTransport, opts ...TTSManagerOption) *TTSManager {
	t := &TTSManager{
		clientState:     clientState,
		serverTransport: serverTransport,
		ttsQueue:        util.NewQueue[TTSQueueItem](10),
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// 启动TTS队列消费协程
func (t *TTSManager) Start(ctx context.Context) {
	t.processTTSQueue(ctx)
}

func (t *TTSManager) processTTSQueue(ctx context.Context) {
	for {
		item, err := t.ttsQueue.Pop(ctx, 0) // 阻塞式
		if err != nil {
			if err == util.ErrQueueCtxDone {
				return
			}
			continue
		}
		if item.onStartFunc != nil {
			item.onStartFunc()
		}
		err = t.handleTts(item.ctx, item.llmResponse)
		if item.onEndFunc != nil {
			item.onEndFunc(err)
		}
	}
}

func (t *TTSManager) ClearTTSQueue() {
	t.ttsQueue.Clear()
}

// 处理文本内容响应（异步 TTS 入队）
func (t *TTSManager) handleTextResponse(ctx context.Context, llmResponse llm_common.LLMResponseStruct, isSync bool) error {
	if llmResponse.Text == "" {
		return nil
	}

	ttsQueueItem := TTSQueueItem{ctx: ctx, llmResponse: llmResponse}
	endChan := make(chan bool, 1)
	ttsQueueItem.onEndFunc = func(err error) {
		select {
		case endChan <- true:
		default:
		}
	}

	t.ttsQueue.Push(ttsQueueItem)

	if isSync {
		timer := time.NewTimer(30 * time.Second)
		defer timer.Stop()
		select {
		case <-endChan:
			return nil
		case <-ctx.Done():
			return fmt.Errorf("TTS 处理上下文已取消")
		case <-timer.C:
			return fmt.Errorf("TTS 处理超时")
		}
	}

	return nil
}

// 同步 TTS 处理
func (t *TTSManager) handleTts(ctx context.Context, llmResponse llm_common.LLMResponseStruct) error {
	if llmResponse.Text == "" {
		return nil
	}

	// 使用带上下文的TTS处理
	outputChan, err := t.clientState.TTSProvider.TextToSpeechStream(ctx, llmResponse.Text, t.clientState.OutputAudioFormat.SampleRate, t.clientState.OutputAudioFormat.Channels, t.clientState.OutputAudioFormat.FrameDuration)
	if err != nil {
		log.Errorf("生成 TTS 音频失败: %v", err)
		return fmt.Errorf("生成 TTS 音频失败: %v", err)
	}

	if err := t.serverTransport.SendSentenceStart(llmResponse.Text); err != nil {
		log.Errorf("发送 TTS 文本失败: %s, %v", llmResponse.Text, err)
		return fmt.Errorf("发送 TTS 文本失败: %s, %v", llmResponse.Text, err)
	}

	// 发送音频帧
	if err := t.SendTTSAudio(ctx, outputChan, llmResponse.IsStart); err != nil {
		log.Errorf("发送 TTS 音频失败: %s, %v", llmResponse.Text, err)
		return fmt.Errorf("发送 TTS 音频失败: %s, %v", llmResponse.Text, err)
	}

	if err := t.serverTransport.SendSentenceEnd(llmResponse.Text); err != nil {
		log.Errorf("发送 TTS 文本失败: %s, %v", llmResponse.Text, err)
		return fmt.Errorf("发送 TTS 文本失败: %s, %v", llmResponse.Text, err)
	}

	return nil
}

func (t *TTSManager) SendTTSAudio(ctx context.Context, audioChan chan []byte, isStart bool) error {
	// 步骤1: 收集前三帧（或更少）
	preBuffer := make([][]byte, 0, 3)
	preBufferCount := 0

	totalFrames := preBufferCount // 跟踪已发送的总帧数

	isStatistic := true
	//首次发送180ms音频, 根据outputAudioFormat.FrameDuration计算
	firstFrameCount := 60 / t.clientState.OutputAudioFormat.FrameDuration
	if firstFrameCount > 20 || firstFrameCount < 3 {
		firstFrameCount = 5
	}
	// 收集前180ms音频
	for totalFrames < firstFrameCount {
		select {
		case <-ctx.Done():
			log.Debugf("SendTTSAudio context done, exit, totalFrames: %d", totalFrames)
			return nil
		default:
			select {
			case frame, ok := <-audioChan:
				if isStart && isStatistic {
					log.Debugf("从接收音频结束 asr->llm->tts首帧 整体 耗时: %d ms", t.clientState.GetAsrLlmTtsDuration())
					isStatistic = false
				}
				if !ok {
					// 通道已关闭，发送已收集的帧并返回
					for _, f := range preBuffer {
						if err := t.serverTransport.SendAudio(f); err != nil {
							return fmt.Errorf("发送 TTS 音频 len: %d 失败: %v", len(f), err)
						}
					}
					return nil
				}
				select {
				case <-ctx.Done():
					log.Debugf("SendTTSAudio context done, exit, totalFrames: %d", totalFrames)
					return nil
				default:
					if err := t.serverTransport.SendAudio(frame); err != nil {
						return fmt.Errorf("发送 TTS 音频 len: %d 失败: %v", len(frame), err)
					}
					log.Debugf("发送 TTS 音频: %d 帧, len: %d", totalFrames, len(frame))
					totalFrames++
				}
			case <-ctx.Done():
				// 上下文已取消
				log.Infof("SendTTSAudio context done, exit, totalFrames: %d", totalFrames)
				return nil
			}
		}
	}

	// 步骤3: 设置定时器，每60ms发送一帧
	ticker := time.NewTicker(time.Duration(t.clientState.OutputAudioFormat.FrameDuration) * time.Millisecond)
	defer ticker.Stop()

	// 循环处理剩余帧
	for {
		select {
		case <-ticker.C:
			// 时间到，尝试获取并发送下一帧
			select {
			case frame, ok := <-audioChan:
				if !ok {
					// 通道已关闭，所有帧已处理完毕
					return nil
				}

				select {
				case <-ctx.Done():
					log.Debugf("SendTTSAudio context done, exit")
					return nil
				default:
					// 发送当前帧
					if err := t.serverTransport.SendAudio(frame); err != nil {
						return fmt.Errorf("发送 TTS 音频 len: %d 失败: %v", len(frame), err)
					}
					totalFrames++
					//log.Debugf("发送 TTS 音频: %d 帧, len: %d", totalFrames, len(frame))
				}
			default:
				// 没有帧可获取，等待下一个周期
			}
		case <-ctx.Done():
			// 上下文已取消
			log.Infof("SendTTSAudio context done, exit, totalFrames: %d", totalFrames)
			return nil
		}
	}
}
