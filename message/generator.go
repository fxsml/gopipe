package message

import "github.com/fxsml/gopipe"

type Generator interface {
	gopipe.Generator[*Message]
}
