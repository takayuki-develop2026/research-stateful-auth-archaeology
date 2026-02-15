package worker

type RouteDecision struct {
	RouteID  string
	Provider string
	Model    string
}

func DecideRoute(mode int) RouteDecision {
	switch mode {
	case 1:
		return RouteDecision{RouteID: "r1", Provider: "stub", Model: "stub"}
	default:
		return RouteDecision{RouteID: "r0", Provider: "stub", Model: "stub"}
	}
}