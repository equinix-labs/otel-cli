package cmd

import (
	"context"
	"testing"
)

func TestLoadTraceparent(t *testing.T) {
	file := "/dev/null"
	ctx := context.Background()

	// TODO:
	//    * set env, see if it comes through
	//    * set env and file, make sure the right one comes out
	//    * clear env, set file, check
	// etc
	loadTraceparent(ctx, file)
}
