// Package middleware provides cross-cutting message.Middleware for Router,
// UnmarshalPipe, and MarshalPipe: CorrelationID, Deadline, Recover, and
// ValidateRequired.
//
// Contract: shipped middleware in this package may only read or modify a
// [message.Message]'s Attributes and locals — never Data. Router dispatches
// purely by CE type and never requires Data to be in any particular state;
// individual handlers decide independently whether to marshal (see
// [message.CommandHandlerConfig]). Middleware that depended on Data's
// concrete type would break that independence, since Router-level .Use()
// middleware runs across all handlers on a router regardless of their
// marshal setting. A per-handler concern needing typed Data — such as
// deriving the CE subject from a typed value — belongs in
// [message.CommandHandlerConfig.Subject], not in a Router-level middleware.
package middleware
