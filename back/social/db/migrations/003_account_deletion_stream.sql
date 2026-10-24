-- Older Social versions could mark a group complete after scheduling Stream
-- removal in the background. Reopen those jobs because their Stream result was
-- never durably recorded; retrying remove_members is safe and idempotent.
ALTER TABLE social.account_deletion_groups
    ADD COLUMN IF NOT EXISTS stream_removed boolean NOT NULL DEFAULT false;

UPDATE social.account_deletion_groups
SET status = 'pending', completed_at = NULL, updated_at = now()
WHERE status = 'complete' AND stream_removed = false;

UPDATE social.account_deletion_users AS users
SET status = 'pending', completed_at = NULL, updated_at = now()
WHERE users.status = 'complete'
  AND EXISTS (
      SELECT 1
      FROM social.account_deletion_groups AS groups
      WHERE groups.user_id = users.user_id AND groups.status = 'pending'
  );
