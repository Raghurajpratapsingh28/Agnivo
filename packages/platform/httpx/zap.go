package httpx

import "go.uber.org/zap"

// Thin aliases so response.go does not import zap directly at call sites.
func zapError(err error) zap.Field         { return zap.Error(err) }
func zapInt(k string, v int) zap.Field     { return zap.Int(k, v) }
func zapString(k, v string) zap.Field      { return zap.String(k, v) }
