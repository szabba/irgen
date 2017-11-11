// Code generated by irgen; DO NOT EDIT.

package types

type Named struct {
	Name string
	Args []Type
}
type Function struct {
	Arg, Output Type
}

func (Type *Named) FeedTo(consumer TypeConsumer)    { consumer.Named(Type.Name, Type.Args) }
func (Type *Function) FeedTo(consumer TypeConsumer) { consumer.Function(Type.Arg, Type.Output) }