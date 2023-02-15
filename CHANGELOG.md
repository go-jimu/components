# Change log

## v0.2.5

- Fixed: bad method name

## v0.2.3

### Changes

- Updated: Golang to version 1.19
- Enhanced: Add `FromContextAsHelper` to convert `Logger` from context as `Helper`
- Enhanced: Add `SetDefault` to change `Default` behavior

## v0.2.2

### Fixed

- 移除了`Mediator.Dispatch`中的`context.Context`参数：我们意识到Mediator一般是异步模式，而`Context`的生成一个同步请求当中，当同步请求完成时则触发`CancelFunc`，这就导致异步处理的事件不一定可以完成

## v0.2.1

### Fixed

- 修复`logger.Warn/Warnf`的调用深度不一致的问题
- `config`模块调整默认日志打印等级