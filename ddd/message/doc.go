// Package message provides protobuf-first integration messages for direct,
// non-transactional communication across bounded contexts or services. Domain
// facts inside one bounded context belong in ddd/event.
//
// Kind is the semantic contract that routes a message to its handlers and
// resolves its payload type. Providers map Kind, ID, OccurredAt, Key, and
// Headers onto their own broker envelopes, such as topics and record headers.
//
// Subscriber registers handlers. The broker runtime (polling,
// acknowledgement, offset commit, retry, redelivery, dead-letter routing) is
// the provider's.
//
// Failures come in two layers:
//
//   - Handler.Handle returns a message-level failure for one delivered
//     message. Each provider documents its policy for these: retry,
//     dead-letter, drop, or record.
//   - Runner.Run returns a runtime-level failure: the provider loop cannot
//     continue safely. Context cancellation or expiration is a normal shutdown
//     and returns nil, matching taskqueue.Runner.
package message
