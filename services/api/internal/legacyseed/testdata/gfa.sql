CREATE TABLE gfa_content_idea (id bigint PRIMARY KEY, kind text, title text, note text, created_by_account_id bigint);
INSERT INTO gfa_content_idea VALUES (1,'game','sensitive-fake-title','fixture-secret',91), (2,'unexpected-private-kind','another-private-title','fixture-private-note',92);
CREATE TABLE gfa_admin_account (id bigint, password_hash text);
INSERT INTO gfa_admin_account VALUES (91,'synthetic-password-do-not-export');
