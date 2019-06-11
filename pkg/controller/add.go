package controller

import (
	"github.com/wantedly/rigger/pkg/controller/dstsecret"
	"github.com/wantedly/rigger/pkg/controller/plan"
	"github.com/wantedly/rigger/pkg/controller/srcsecret"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, plan.Add)
	AddToManagerFuncs = append(AddToManagerFuncs, srcsecret.Add)
	AddToManagerFuncs = append(AddToManagerFuncs, dstsecret.Add)
}
