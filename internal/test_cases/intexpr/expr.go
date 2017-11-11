// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package intexpr

//go:generate irgen -v -out ref.go Expr ExprConsumer

type Expr interface {
	FeedTo(cons ExprConsumer)
}

type ExprConsumer interface {
	Lit(N int)
	Var(Name string)
	Add(Left, Right Expr)
	Sub(Left, Right Expr)
	Mul(Left, Right Expr)
}
