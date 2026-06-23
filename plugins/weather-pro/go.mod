// Module is separate from the parent so plugin authors see the public SDK surface
// the same way third-party plugins will: only pkg/leahplugin is reachable; internal/
// stays off-limits by Go's internal rule.
module github.com/trilam/leah/plugins/weather-pro

go 1.25.0

require github.com/trilam/leah v0.0.0

replace github.com/trilam/leah => ../..
