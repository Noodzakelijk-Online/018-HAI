package rbac

import "testing"

func TestOwnerHasAllPermissions(t *testing.T) {
	for _, p := range []Permission{PermRead, PermWrite, PermExecute, PermApprove, PermAdmin} {
		if !Can(RoleOwner, p) {
			t.Fatalf("owner should have %s", p)
		}
	}
}

func TestViewerIsReadOnly(t *testing.T) {
	if !Can(RoleViewer, PermRead) {
		t.Fatalf("viewer should read")
	}
	for _, p := range []Permission{PermWrite, PermExecute, PermApprove, PermAdmin} {
		if Can(RoleViewer, p) {
			t.Fatalf("viewer must not have %s", p)
		}
	}
}

func TestOperatorCanOperateButCannotMakeOwnerDecisions(t *testing.T) {
	if !Can(RoleOperator, PermWrite) || !Can(RoleOperator, PermExecute) {
		t.Fatalf("operator should write and execute bounded operations")
	}
	for _, p := range []Permission{PermApprove, PermAdmin} {
		if Can(RoleOperator, p) {
			t.Fatalf("operator must not have %s", p)
		}
	}
}

func TestUnknownRoleGrantsNothing(t *testing.T) {
	if Can(Role("hacker"), PermRead) {
		t.Fatalf("unknown role must grant nothing")
	}
	if IsRole("hacker") {
		t.Fatalf("unknown role should not be recognized")
	}
}

func TestRolesAndPermissionsSorted(t *testing.T) {
	roles := Roles()
	if len(roles) != 3 || roles[0] != RoleOperator {
		t.Fatalf("roles not sorted/complete: %v", roles)
	}
	if perms := Permissions(RoleViewer); len(perms) != 1 || perms[0] != PermRead {
		t.Fatalf("viewer permissions wrong: %v", perms)
	}
}
