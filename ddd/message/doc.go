// Package message provides protobuf-first integration messages for direct,
// non-transactional communication across bounded contexts or services. Domain
// facts inside one bounded context belong in ddd/event.
//
// Application code depends on this package only: it publishes through
// Publisher and subscribes Handler values through Subscriber. A provider
// adapter, such as github.com/go-jimu/contrib/message/kafka, implements those
// interfaces and is imported in runtime composition only.
//
// Read by task:
//   - Publishing a message: "Kind", "Publishing".
//   - Writing a handler: "Kind", "Handlers", "Middleware", "Failures".
//   - Composing a consumer: "Consuming".
//   - Writing a provider adapter: "Providers", "Failures".
//
// # Kind
//
// Kind is the semantic contract that routes a message to its handlers and
// names its payload type. For protobuf payloads, derive it with KindOf, which
// returns the payload's protobuf full name, such as
// "user.integration.v1.UserRegisteredV1". Use the same KindOf call on every
// side: when constructing the message, when registering the payload factory,
// and in Handler.Listening. Version a contract by versioning the protobuf
// package or message name, which changes the Kind with it.
//
// Write a Kind by hand only when it must name something other than one
// generated message type: a payload type shared by several facts, such as a
// well-known wrapper type, or a name fixed by an existing external contract.
// Declare that Kind once as a constant and use the constant on every side.
//
// # Publishing
//
// Build the message at the publication boundary and publish it:
//
//	payload := &userv1.UserRegisteredV1{UserId: user.ID}
//	msg, err := message.New(message.KindOf(payload), payload, message.WithKey(user.ID))
//	if err != nil {
//		return err
//	}
//	return publisher.Publish(ctx, msg)
//
// # Consuming
//
// Compose a consumer in this order, before it starts consuming:
//
//  1. Register a payload factory in a PayloadRegistry for every Kind a
//     handler listens to; the provider decodes a payload before routing it.
//  2. Pass the registry to the provider as its PayloadResolver, for example
//     kafka.WithPayloadResolver.
//  3. Subscribe each Handler, wrapped with Chain when it takes Middleware.
//  4. Run the provider when it implements Runner.
//
// Step 1 for one Kind:
//
//	registry := message.NewPayloadRegistry()
//	err := registry.Register(message.KindOf(&userv1.UserRegisteredV1{}), func() proto.Message {
//		return &userv1.UserRegisteredV1{}
//	})
//
// # Handlers
//
// A Handler type-asserts msg.Payload() to its generated type and returns an
// error when the assertion fails.
//
// Handle returns nil when the message is fully handled, and the provider marks
// delivery complete. Return a non-nil error only when the provider should
// apply its failure policy, such as retry or dead-letter. Handle a business
// outcome that redelivery cannot change, such as a duplicate or a rejected
// request, inside the handler and return nil. Providers may deliver a message
// more than once, so make handlers idempotent, for example keyed by
// Message.ID.
//
// # Middleware
//
// Chain wraps a Handler's Handle with Middleware, for logging, metrics, or
// tracing, and keeps its Listening; subscribe the result in its place:
//
//	subscriber.Subscribe(message.Chain(handler, message.Logging(logger)))
//
// # Failures
//
// Failures come in two layers:
//
//   - Handler.Handle returns a message-level failure for one delivered
//     message. Each provider documents its failure policy for these: retry,
//     dead-letter, drop, or record.
//   - Runner.Run returns a runtime-level failure: the provider loop cannot
//     continue safely. Context cancellation or expiration is a normal shutdown
//     and returns nil, matching taskqueue.Runner.
//
// Each function documents the Err* values it returns; compare them with
// errors.Is.
//
// # Providers
//
// A provider adapter:
//   - maps Kind, ID, OccurredAt, Key, and Headers onto its broker envelope,
//     such as a topic and record headers;
//   - owns the broker runtime: polling, acknowledgement, offset commit, retry,
//     redelivery, and dead-letter routing, started by Runner.Run;
//   - decodes each payload into the target its PayloadResolver returns, then
//     routes the message, usually through a Router, which also implements
//     Subscriber;
//   - applies its failure policy to a Handle error and logs the failure at the
//     level that policy implies, such as Warn for a retry and Error for a lost
//     message; Logging records it at Debug only.
package message
