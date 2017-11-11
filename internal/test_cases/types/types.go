// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package types

//go:generate irgen -v -out ref.go Type TypeConsumer

type Type interface {
	FeedTo(cons TypeConsumer)
}

type TypeConsumer interface {
	Named(Name string, Args []Type)
	Function(Arg, Output Type)
}
