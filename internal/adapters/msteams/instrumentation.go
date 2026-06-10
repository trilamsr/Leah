package msteams

import (
	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/obs/connectadapter"
)

const provider = "msteams"

func Metrics(r *obs.Registry) *connectadapter.Metrics { return connectadapter.For(provider, r) }

func RegisterMetrics(r *obs.Registry) { Metrics(r).Register() }
