package flights

import (
	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/obs/connectadapter"
)

const provider = "flights"

func Metrics(r *telemetry.Registry) *connectadapter.Metrics { return connectadapter.For(provider, r) }

func RegisterMetrics(r *telemetry.Registry) { Metrics(r).Register() }
