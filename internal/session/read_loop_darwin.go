package session

// Darwin can report PTY EOF as soon as the session leader exits, discarding
// output still queued on the slave. Read ahead while a delivery is pending so
// final output reaches OnData before the process is reaped.
func (s *Session) readLoop(onData func([]byte) bool) {
	defer close(s.readDone)
	const queuedChunks = 32 // At most 1 MiB of output can wait for acknowledgement.
	chunks := make(chan []byte, queuedChunks)
	producerDone := make(chan struct{})
	go func() {
		defer close(producerDone)
		defer close(chunks)
		buffer := make([]byte, readBufferSize)
		for {
			n, err := s.readTerminal(buffer)
			if n > 0 {
				chunk := append([]byte(nil), buffer[:n]...)
				select {
				case chunks <- chunk:
				case <-s.readStop:
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	defer func() {
		s.stopReader()
		<-producerDone
	}()
	for chunk := range chunks {
		if onData != nil && !onData(chunk) {
			s.stopAfterOutputFailure()
			return
		}
	}
}
