package logger

import "context"

type ctxKey struct{}

// InContext 将Logger存入Context。
func InContext(ctx context.Context, log Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, log)
}

// FromContext 从Context中提取Logger，如果不存在则返回默认的Logger。
func FromContext(ctx context.Context) Logger {
	if l, ok := ctx.Value(ctxKey{}).(Logger); ok {
		return l
	}
	return Default()
}

// FromContextAsHelper 从Context中提取Logger，返回Helper对象。
func FromContextAsHelper(ctx context.Context) *Helper {
	log := FromContext(ctx)
	return NewHelper(log).WithContext(ctx)
}
