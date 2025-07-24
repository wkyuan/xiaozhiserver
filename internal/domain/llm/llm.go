package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode"
	"xiaozhi-esp32-server-golang/internal/domain/llm/common"

	log "xiaozhi-esp32-server-golang/logger"

	"github.com/cloudwego/eino/schema"
)

// 句子结束的标点符号
var sentenceEndPunctuation = []rune{'.', '。', '!', '！', '?', '？', '\n'}

// 句子暂停的标点符号（可以作为长句子的断句点）
var sentencePausePunctuation = []rune{',', '，', ';', '；', ':', '：'}

// 判断一个字符是否为句子结束的标点符号
func isSentenceEndPunctuation(r rune) bool {
	for _, p := range sentenceEndPunctuation {
		if r == p {
			return true
		}
	}
	return false
}

// 判断一个字符是否为句子暂停的标点符号
func isSentencePausePunctuation(r rune) bool {
	for _, p := range sentencePausePunctuation {
		if r == p {
			return true
		}
	}
	return false
}

// HandleLLMWithContextAndTools 使用上下文控制来处理LLM响应（兼容带工具和不带工具）
func HandleLLMWithContextAndTools(ctx context.Context, llmProvider LLMProvider, dialogue []*schema.Message, tools []*schema.ToolInfo, sessionID string) (chan common.LLMResponseStruct, error) {
	var (
		llmResponse interface{}
	)
	llmResponse = llmProvider.ResponseWithContext(ctx, sessionID, dialogue, tools)

	sentenceChannel := make(chan common.LLMResponseStruct, 2)
	startTs := time.Now().UnixMilli()
	var firstFrame bool
	fullText := ""
	var buffer bytes.Buffer // 用于累积接收到的内容
	isFirst := true

	go func() {
		defer func() {
			log.Debugf("full Response with %d tools, fullText: %s", len(tools), fullText)
			close(sentenceChannel)
		}()
		msgChan, ok := llmResponse.(chan *schema.Message)
		if !ok {
			log.Errorf("llmResponse 断言为 chan *schema.Message 失败")
			return
		}
		for {
			select {
			case <-ctx.Done():
				log.Infof("上下文已取消，停止LLM响应处理: %v, context done, exit", ctx.Err())
				return
			default:
				select {
				case message, ok := <-msgChan:
					if !ok {
						remaining := buffer.String()
						if remaining != "" {
							log.Infof("处理剩余内容: %s", remaining)
							fullText += remaining
							sentenceChannel <- common.LLMResponseStruct{
								Text:  remaining,
								IsEnd: true,
							}
						} else {
							sentenceChannel <- common.LLMResponseStruct{
								Text:  "",
								IsEnd: true,
							}
						}
						return
					}
					if message == nil {
						break
					}
					byteMessage, _ := json.Marshal(message)
					log.Infof("收到message: %s", string(byteMessage))
					if message.Content != "" {
						fullText += message.Content
						buffer.WriteString(message.Content)
						if containsSentenceSeparator(message.Content, isFirst) {
							sentences, remaining := extractSmartSentences(buffer.String(), 5, 100, isFirst)
							if len(sentences) > 0 {
								for _, sentence := range sentences {
									if sentence != "" {
										if !firstFrame {
											firstFrame = true
											log.Infof("耗时统计: llm工具首句: %d ms", time.Now().UnixMilli()-startTs)
										}
										log.Infof("处理完整句子: %s", sentence)
										sentenceChannel <- common.LLMResponseStruct{
											Text:    sentence,
											IsStart: isFirst,
											IsEnd:   false,
										}
										if isFirst {
											isFirst = false
										}
									}
								}
							}
							buffer.Reset()
							buffer.WriteString(remaining)
							if isFirst {
								isFirst = false
							}
						}
					}
					// 工具调用响应（假设 ToolCalls 字段）
					if message.ToolCalls != nil && len(message.ToolCalls) > 0 {
						log.Infof("处理工具调用: %+v", message.ToolCalls)
						sentenceChannel <- common.LLMResponseStruct{
							ToolCalls: message.ToolCalls,
							IsStart:   isFirst,
							IsEnd:     false,
						}
					}
				default:

				}
			}
		}
	}()
	return sentenceChannel, nil
}

// 判断字符串是否为数字加点号格式（如"1."、"2."等）
func isNumberWithDot(s string) bool {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) < 2 || trimmed[len(trimmed)-1] != '.' {
		return false
	}

	for i := 0; i < len(trimmed)-1; i++ {
		if !unicode.IsDigit(rune(trimmed[i])) {
			return false
		}
	}
	return true
}

// 从文本中提取完整的句子
// 返回完整句子的切片和剩余的未完成内容
func extractCompleteSentences(text string) ([]string, string) {
	if text == "" {
		return []string{}, ""
	}

	var sentences []string
	var currentSentence bytes.Buffer

	runes := []rune(text)
	lastIndex := len(runes) - 1

	for i, r := range runes {
		currentSentence.WriteRune(r)

		// 判断句子是否结束
		if isSentenceEndPunctuation(r) {
			// 如果是句子结束标点
			sentence := strings.TrimSpace(currentSentence.String())
			if sentence != "" {
				sentences = append(sentences, sentence)
			}
			currentSentence.Reset()
		} else if i == lastIndex {
			// 如果是最后一个字符但不是句子结束标点，保留在remaining中
			break
		}
	}

	// 当前未完成的句子作为remaining返回
	remaining := currentSentence.String()
	return sentences, strings.TrimSpace(remaining)
}

// ConvertMCPToolsToEinoTools 将MCP工具转换为Eino ToolInfo格式
func ConvertMCPToolsToEinoTools(ctx context.Context, mcpTools map[string]interface{}) ([]*schema.ToolInfo, error) {
	var einoTools []*schema.ToolInfo

	for toolName, mcpTool := range mcpTools {
		// 尝试获取工具信息
		if invokableTool, ok := mcpTool.(interface {
			Info(context.Context) (*schema.ToolInfo, error)
		}); ok {
			toolInfo, err := invokableTool.Info(ctx)
			if err != nil {
				log.Errorf("获取工具 %s 信息失败: %v", toolName, err)
				continue
			}
			einoTools = append(einoTools, toolInfo)
		} else {
			log.Warnf("工具 %s 不支持Info接口，跳过转换", toolName)
		}
	}

	log.Infof("成功转换了 %d 个MCP工具为Eino工具", len(einoTools))
	return einoTools, nil
}
