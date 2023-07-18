package otlpclient

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestErrorLists(t *testing.T) {
	now := time.Now()

	for _, tc := range []struct {
		call func(context.Context) context.Context
		want ErrorList
	}{
		{
			call: func(ctx context.Context) context.Context {
				err := fmt.Errorf("")
				ctx, _ = SaveError(ctx, now, err)
				return ctx
			},
			want: ErrorList{
				TimestampedError{now, ""},
			},
		},
	} {
		ctx := context.Background()
		ctx = tc.call(ctx)
		list := GetErrorList(ctx)

		if len(list) < len(tc.want) {
			t.Errorf("got %d errors but expected %d", len(tc.want), len(list))
		}

		// TODO: sort?
		if diff := cmp.Diff(list, tc.want); diff != "" {
			t.Errorf("error list mismatch (-want +got):\n%s", diff)
		}

	}
}
