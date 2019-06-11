package plan

import (
	"sync"

	riggerv1beta1 "github.com/wantedly/rigger/pkg/apis/rigger/v1beta1"
)

var Cache = &cache{}

// Data set of secret resource
type cache struct {
	sm sync.Map
}

func (s *cache) Store(name string, plan *riggerv1beta1.Plan) {
	s.sm.Store(name, plan)
}

func (s *cache) Load(name string) (*riggerv1beta1.Plan, bool) {
	v, ok := s.sm.Load(name)
	if !ok {
		return nil, false
	}
	return v.(*riggerv1beta1.Plan), ok
}

func (s *cache) Range(f func(name, plan interface{}) bool) {
	s.sm.Range(f)
}

func (s *cache) Delete(name string) {
	s.sm.Delete(name)
}
