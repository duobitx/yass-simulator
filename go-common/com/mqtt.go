package com

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/m-szalik/goutils"
	"github.com/sirupsen/logrus"
)
import mqtt "github.com/eclipse/paho.mqtt.golang"

const timeout = 5 * time.Second

type mqttFacade struct {
	client                 mqtt.Client
	connectionErrorHandler ConnectionErrorHandler
	ctx                    context.Context
}

func (m *mqttFacade) OnConnectionError(handler ConnectionErrorHandler) {
	m.connectionErrorHandler = handler
}

func (m *mqttFacade) Close() error {
	m.client.Disconnect(10000)
	return nil
}

func (m *mqttFacade) Connect() error {
	if token := m.client.Connect(); token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

func (m *mqttFacade) IsConnected() bool {
	return m.client.IsConnected()
}

func (m *mqttFacade) Subscribe(topic string, handler MessageSubscriptionFunct) error {
	slog.Info("Subscribe", "topic", topic)
	token := m.client.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
		defer msg.Ack()
		go func(message mqtt.Message) {
			opCtx, cancel := context.WithTimeout(m.ctx, 20*time.Second) // TODO timeout
			defer cancel()
			handler(opCtx, message.Topic(), message.Retained(), message.Payload())
		}(msg)
	})

	if !token.WaitTimeout(timeout) {
		return errors.New(fmt.Sprintf("subscription for %s timeout", topic))
	}
	if token.Error() != nil {
		return fmt.Errorf("subscription for %s error - %w", topic, token.Error())
	}
	return nil
}

func (m *mqttFacade) Unsubscribe(topic string) error {
	token := m.client.Unsubscribe(topic)
	if !token.WaitTimeout(timeout) {
		return errors.New(fmt.Sprintf("unsubscribe for %s timeout", topic))
	}
	if token.Error() != nil {
		return fmt.Errorf("unsubscribe for %s error - %w", topic, token.Error())
	}
	return nil
}

func (m *mqttFacade) Publish(_ context.Context, topic string, qos byte, retained bool, payloadObj any) error {
	buff, err := MsgMarshall(payloadObj)
	if err != nil {
		return err
	}
	token := m.client.Publish(topic, qos, retained, buff)
	if !token.WaitTimeout(timeout) {
		return errors.New(fmt.Sprintf("publication to %s mqttTimeout (timeout: %s)", topic, timeout))
	}

	pErr := token.Error()
	if pErr != nil {
		return fmt.Errorf("publication to %s error - %w", topic, pErr)
	}
	return nil
}

type mqttLogger struct {
	lev logrus.Level
}

func (m *mqttLogger) Println(v ...interface{}) {
	logrus.StandardLogger().Log(m.lev, v...)
}

func (m *mqttLogger) Printf(format string, v ...interface{}) {
	logrus.StandardLogger().Log(m.lev, fmt.Sprintf(format, v...))
}

func newMqttLogger(lev logrus.Level) mqtt.Logger {
	return &mqttLogger{
		lev: lev,
	}
}

func NewMqttFacade(ctx context.Context, clientID string) Facade {
	mqtt.DEBUG = mqtt.NOOPLogger{}
	mqtt.WARN = newMqttLogger(logrus.WarnLevel)
	mqtt.ERROR = newMqttLogger(logrus.ErrorLevel)
	mqtt.CRITICAL = newMqttLogger(logrus.ErrorLevel)

	facade := &mqttFacade{
		client: nil,
		ctx:    ctx,
	}
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://" + goutils.Env("MESSAGING_BROKER_HOST_PORT", "messaging:1883"))
	opts.SetClientID(clientID)
	opts.SetAutoReconnect(true)
	opts.SetResumeSubs(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(3 * time.Second)
	opts.SetOrderMatters(false)
	opts.MaxResumePubInFlight = 0
	opts.MaxReconnectInterval = 10 * time.Second
	opts.OnReconnecting = func(client mqtt.Client, options *mqtt.ClientOptions) {
		slog.Default().Info("MQTT: ReConnecting")
	}
	opts.OnConnect = func(client mqtt.Client) {
		slog.Default().Info("MQTT: Connected")
	}
	opts.OnConnectionLost = func(client mqtt.Client, err error) {
		slog.Default().Info("MQTT: Connection lost")
		if facade.connectionErrorHandler != nil {
			facade.connectionErrorHandler(err)
		}
	}

	client := mqtt.NewClient(opts)
	facade.client = client
	return facade
}
