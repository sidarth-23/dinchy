-- +goose Up

ALTER TABLE organisation_roles RENAME COLUMN organisation_id TO organization_id;
ALTER TABLE organisation_members RENAME COLUMN organisation_id TO organization_id;
ALTER TABLE organisation_invitations RENAME COLUMN organisation_id TO organization_id;
ALTER TABLE sessions RENAME COLUMN active_organisation_id TO active_organization_id;
ALTER TABLE app_audit_logs RENAME COLUMN actor_organisation_id TO actor_organization_id;

ALTER TABLE organisations RENAME TO organizations;
ALTER TABLE organisation_roles RENAME TO organization_roles;
ALTER TABLE organisation_role_permissions RENAME TO organization_role_permissions;
ALTER TABLE organisation_members RENAME TO organization_members;
ALTER TABLE organisation_invitations RENAME TO organization_invitations;

-- +goose Down

ALTER TABLE organizations RENAME TO organisations;
ALTER TABLE organization_roles RENAME TO organisation_roles;
ALTER TABLE organization_role_permissions RENAME TO organisation_role_permissions;
ALTER TABLE organization_members RENAME TO organisation_members;
ALTER TABLE organization_invitations RENAME TO organisation_invitations;

ALTER TABLE organisation_roles RENAME COLUMN organization_id TO organisation_id;
ALTER TABLE organisation_members RENAME COLUMN organization_id TO organisation_id;
ALTER TABLE organisation_invitations RENAME COLUMN organization_id TO organisation_id;
ALTER TABLE sessions RENAME COLUMN active_organization_id TO active_organisation_id;
ALTER TABLE app_audit_logs RENAME COLUMN actor_organization_id TO actor_organisation_id;
