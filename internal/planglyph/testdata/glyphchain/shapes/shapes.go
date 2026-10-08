// shapes.go declares one member of every shape class a plan card names, for the glyph-chain test.

package shapes

// Func is a plain function.
func Func() int { return 0 }

// Plain is a struct with no members.
type Plain struct{}

// Struct is a struct with a field and methods of both receiver kinds.
type Struct struct {
	Field int
}

// ValueMethod has a value receiver.
func (s Struct) ValueMethod() int { return s.Field }

// PointerMethod has a pointer receiver.
func (s *Struct) PointerMethod() { s.Field++ }

// Iface is an interface with one method.
type Iface interface {
	IfaceMethod() int
}

// Generic is a generic struct; GenericMethod names it in its receiver, outside the type's own span.
type Generic[E any] struct{ value E }

// GenericMethod returns the held value.
func (g *Generic[E]) GenericMethod() E { return g.value }

// Var is a lone variable.
var Var = 1

// Const is a lone constant.
const Const = 2

var (
	BlockVar     = 3
	PairA, PairB = 4, 5
)

const (
	BlockConst = iota
	NextConst
)
