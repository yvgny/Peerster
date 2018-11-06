package main

import "sync"

type Mail struct {
	mailbox []Message
	mutex   sync.RWMutex
}

type Message struct {
	From string
	To   string
	Text string
}

func newMail() *Mail {
	return &Mail{}
}

func (m *Mail) addMessage(from, to, content string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.mailbox = append(m.mailbox, Message{
		Text: content,
		From: from,
		To:   to,
	})
}

func (m *Mail) Elements() []Message {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	copied := make([]Message, len(m.mailbox))
	copy(copied, m.mailbox)

	return copied
}
