`irgen` is a code generator for the visitees (elements) in the visitor pattern.

Given a file `option.go` in the directory
`$GOPATH/src/some-domain.org/option`
containing

```go
package option

//go:generate irgen Option OptionConsumer

type Option interface {
    FeedTo(consumer OptionConsumer)
}

type OptionConsumer interface {
    Some(X interface{})
    None()
}
```

running `go generate some-domain.org/option` will generate a file named
`option_impl.go` containing something like

```go
package option

type Some struct {
    X interface{}
}
type None struct {
}

func (option Some) FeedTo(consumer OptionConsumer) { consumer.Some(Option.X) }
func (option None) FeedTo(consumer OptionConsumer) { consumer.None() }
```