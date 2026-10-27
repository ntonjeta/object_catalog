package domain

import "time"

// InstanceActorHandle is the fediverse handle of the instance-level actor.
const InstanceActorHandle = "catalog"

// InstanceKey is the RSA signing key pair of the instance actor, persisted as
// a PKCS#8 PEM private key. The public key is derived, never stored.
type InstanceKey struct {
	PrivatePEM string
	CreatedAt  time.Time
}
