DROP TABLE IF EXISTS invitations;
DROP TABLE IF EXISTS rooms;
DROP TABLE IF EXISTS member_roles;
DROP TRIGGER IF EXISTS roles_protect_system_roles ON roles;
DROP FUNCTION IF EXISTS protect_system_roles();
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS members;
DROP TABLE IF EXISTS communities;
