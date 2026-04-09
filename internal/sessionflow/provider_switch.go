package sessionflow

import (
	"cliamp/internal/session"
	"cliamp/internal/source"
)

type ProviderSwitchAction string

const (
	ProviderSwitchNone            ProviderSwitchAction = ""
	ProviderSwitchRestoreHydrated ProviderSwitchAction = "restore_hydrated"
	ProviderSwitchRestoreDeferred ProviderSwitchAction = "restore_deferred"
)

type ProviderSwitchInput struct {
	TargetKey          string
	CurrentProviderKey string
	CurrentOwnerKey    string
	Sessions           map[string]session.HydratedState
}

type ProviderSwitchPlan struct {
	SelectingCurrent        bool
	Action                  ProviderSwitchAction
	Restore                 session.RestorePlan
	BackgroundRefetchSource source.Ref
}

func PlanProviderSwitch(planner session.Planner, input ProviderSwitchInput) ProviderSwitchPlan {
	plan := ProviderSwitchPlan{
		SelectingCurrent: input.TargetKey == input.CurrentProviderKey || input.TargetKey == input.CurrentOwnerKey,
	}
	if input.TargetKey == "" || plan.SelectingCurrent {
		return plan
	}
	cached, ok := input.Sessions[input.TargetKey]
	if !ok {
		return plan
	}
	restore := planner.RestorePlanForHydratedState(cached)
	if !restore.Planned() {
		return plan
	}
	plan.Restore = restore
	switch restore.Mode {
	case session.RestoreHydratedSnapshot:
		plan.Action = ProviderSwitchRestoreHydrated
		if restore.State.IsSourceSession() && planner.CanRestoreSource(restore.State.Source.ProviderKey) {
			plan.BackgroundRefetchSource = restore.State.Source
		}
	case session.RestoreDeferred:
		plan.Action = ProviderSwitchRestoreDeferred
	}
	return plan
}
