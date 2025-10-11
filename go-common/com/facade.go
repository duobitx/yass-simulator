package com

import (
	"context"
	"io"
)

// NewFacade based on environment variables
func NewFacade(ctx context.Context, clientID string) Facade {
	return NewMqttFacade(ctx, clientID)
}

type MessageSubscriptionFunct func(sCtx context.Context, topic string, retained bool, data []byte)

type ConnectionErrorHandler func(err error)
type Facade interface {
	io.Closer
	Connect() error
	IsConnected() bool
	Subscribe(topic string, handler MessageSubscriptionFunct) error
	Unsubscribe(topic string) error
	Publish(ctx context.Context, topic string, qos byte, retained bool, payloadObj any) error
	OnConnectionError(handler ConnectionErrorHandler)
}
