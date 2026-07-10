package gmail

import (
	"github.com/trilam/leah/internal/platform/telemetry"
	"github.com/trilam/leah/internal/platform/telemetry/connectadapter"
)

const provider = "gmail"

func Metrics(r *telemetry.Registry) *connectadapter.Metrics { return connectadapter.For(provider, r) }

func RegisterMetrics(r *telemetry.Registry) { Metrics(r).Register() }
