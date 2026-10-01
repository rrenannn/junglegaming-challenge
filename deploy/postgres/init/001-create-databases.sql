CREATE ROLE jungle_app LOGIN PASSWORD 'jungle-local-password';
CREATE DATABASE jungle_gaming OWNER jungle_app;

CREATE ROLE keycloak_app LOGIN PASSWORD 'keycloak-local-password';
CREATE DATABASE keycloak OWNER keycloak_app;
