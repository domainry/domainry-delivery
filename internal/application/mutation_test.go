package application

import (
	"context"
	"testing"
	"time"

	"github.com/domainry/domainry-delivery/internal/domain"
	commanddomain "github.com/domainry/domainry-delivery/internal/domain/command"
)

func TestLocalExecutionRoleIsDerivedFromTheCommandCatalog(t *testing.T) {
	service := NewService(Ports{})
	service.now = func() time.Time { return time.Unix(1, 0).UTC() }
	tests := []struct {
		name       string
		command    string
		target     commanddomain.Target
		permission string
		want       domain.ActorKind
	}{
		{name: "local agent", command: commanddomain.FeatureDiscoveryReplace, target: commanddomain.TargetProduct, permission: PermissionProductWrite, want: domain.ActorAgent},
		{name: "human decision", command: commanddomain.FeatureConfirm, target: commanddomain.TargetProduct, permission: PermissionProductWrite, want: domain.ActorHuman},
		{name: "system adapter", command: commanddomain.ProductFoundationStarted, target: commanddomain.TargetProduct, permission: PermissionDeploymentRecord, want: domain.ActorSystem},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := WithTrustedPrincipal(context.Background(), "workspace-1", domain.Actor{ID: "verdent-user", Kind: domain.ActorHuman}, test.permission)
			actor, err := executeMutation(ctx, service, commandScope{Target: test.target, WorkspaceID: "workspace-1", FingerprintIdentity: []string{"workspace-1", "test"}}, domain.Command{
				ClientID: "client-1", Type: test.command,
			}, nil, func(command domain.Command, _ Mutation, _ time.Time) (domain.Actor, error) {
				return command.Actor, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if actor.ID != "verdent-user" || actor.Kind != test.want {
				t.Fatalf("actor=%+v want id=verdent-user kind=%s", actor, test.want)
			}
		})
	}
}
