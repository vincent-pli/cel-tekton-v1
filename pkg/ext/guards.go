package ext

import (
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
	"github.com/google/cel-go/interpreter/functions"
)

func callInListStrStrOutInt(fn func([]string, string) (int, error)) functions.BinaryOp {
	return func(val, arg ref.Val) ref.Val {
		vVal, ok := val.(traits.Lister)
		if !ok {
			return types.MaybeNoSuchOverloadErr(val)
		}
		argVal, ok := arg.(types.String)
		if !ok {
			return types.MaybeNoSuchOverloadErr(arg)
		}
		it := vVal.Iterator()
		i := 0
		source := []string{}
		for ; it.HasNext() == types.True; i++ {
			item := it.Next()
			vItem, ok := item.(types.String)
			if !ok {
				return types.MaybeNoSuchOverloadErr(arg)
			}

			source = append(source, string(vItem))
		}

		out, err := fn(source, string(argVal))
		if err != nil {
			return types.NewErr(err.Error())
		}
		return types.DefaultTypeAdapter.NativeToValue(out)
	}
}
