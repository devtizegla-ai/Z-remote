package http

import "context"

type contextKey string

const (
	ContextUserID  contextKey = "user_id"
	ContextDeviceID contextKey = "device_id"
)

func UserIDFromContext(ctx context.Context) (string, bool) {
	value, ok := ctx.Value(ContextUserID).(string)
	return value, ok
}

func DeviceIDFromContext(ctx context.Context) (string, bool) {
	value, ok := ctx.Value(ContextDeviceID).(string)
	return value, ok
}

