package session

func (s *Session) readLoop(onData func([]byte) bool) {
	defer close(s.readDone)
	buffer := make([]byte, readBufferSize)
	for {
		n, err := s.readTerminal(buffer)
		if n > 0 && onData != nil && !onData(buffer[:n]) {
			s.stopAfterOutputFailure()
			return
		}
		if err != nil {
			return
		}
	}
}
