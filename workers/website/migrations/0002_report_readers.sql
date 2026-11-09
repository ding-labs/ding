-- Keeping an account out of metrics must never grant it report access.
CREATE TABLE report_readers (account TEXT PRIMARY KEY);
