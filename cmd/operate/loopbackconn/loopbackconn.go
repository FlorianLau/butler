package loopbackconn

import (
	"context"
	"fmt"

	"github.com/itchio/butler/butlerd"

	"github.com/itchio/wharf/state"
)

//

type NotificationHandler func(ctx context.Context, method string, params interface{}) error
type CallHandler func(ctx context.Context, method string, params interface{}, result interface{}) error

var NoopNotificationHandler NotificationHandler = func(ctx context.Context, method string, params interface{}) error {
	return nil
}

type LoopbackConn interface {
	butlerd.Conn

	OnNotification(method string, handler NotificationHandler)
	OnCall(method string, handler CallHandler)
}

type loopbackConn struct {
	consumer             *state.Consumer
	notificationHandlers map[string]NotificationHandler
	callHandlers         map[string]CallHandler
}

func New(consumer *state.Consumer) LoopbackConn {
	lc := &loopbackConn{
		consumer:             consumer,
		notificationHandlers: make(map[string]NotificationHandler),
		callHandlers:         make(map[string]CallHandler),
	}

	lc.OnNotification("Log", func(ctx context.Context, method string, params interface{}) error {
		log := params.(*butlerd.LogNotification)
		lc.consumer.OnMessage(string(log.Level), log.Message)
		return nil
	})

	return lc
}

var _ butlerd.Conn = (*loopbackConn)(nil)

func (lc *loopbackConn) OnNotification(method string, handler NotificationHandler) {
	lc.notificationHandlers[method] = handler
}

func (lc *loopbackConn) Notify(ctx context.Context, method string, params interface{}) error {
	if h, ok := lc.notificationHandlers[method]; ok {
		return h(ctx, method, params)
	}
	return nil
}

func (lc *loopbackConn) OnCall(method string, handler CallHandler) {
	lc.callHandlers[method] = handler
}

func (lc *loopbackConn) Call(ctx context.Context, method string, params interface{}, result interface{}) error {
	if h, ok := lc.callHandlers[method]; ok {
		return h(ctx, method, params, result)
	}
	return fmt.Errorf("No handler registered for method (%s)", method)
}

func (lc *loopbackConn) Close() error {
	// no-op
	return nil
}
