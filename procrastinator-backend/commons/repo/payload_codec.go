package repo

// PayloadCodec is the contract for marshaling and unmarshaling entity payload
// data. The infra/postgres codec implements this interface; the generic
// repository consumes it to read and write entity data without knowing the
// concrete codec implementation.
//
// kind identifies the entity kind (e.g. "document", "asset"). entity and
// target are the domain entity values (any) that the codec maps to/from the
// payload JSON bytes.
type PayloadCodec interface {
	// Marshal encodes the entity's data portion into JSON payload bytes.
	Marshal(kind string, entity any) ([]byte, error)
	// Unmarshal decodes JSON payload bytes into the target entity's data portion.
	Unmarshal(kind string, data []byte, target any) error
}
