package chat

func (s *ChatSession) StopSpeaking(isSendTtsStop bool) {
	s.clientState.CancelSessionCtx()
	s.llmManager.ClearLLMResponseQueue()
	s.ClearChatTextQueue()
	if isSendTtsStop {
		s.serverTransport.SendTtsStop()
	}
}
