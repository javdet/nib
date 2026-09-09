-- chat_attachments rows carry the only index of what exists under
-- data/attachments, and both FKs are ON DELETE CASCADE -- so deleting a dialog,
-- or rewinding a conversation with DeleteMessagesAfterSeq on every retry,
-- destroys that index before anything can act on it. There is no os.RemoveAll
-- anywhere in the backend, so the bytes stay on disk forever.
--
-- The trigger fires on cascades too, which is the point: the row's path is
-- captured on the way out, and a Go sweeper drains the queue.

CREATE TABLE attachment_files_to_delete (
    id        bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    path      text NOT NULL,
    dialog_id uuid NOT NULL,
    queued_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_attachment_files_to_delete_queued
    ON attachment_files_to_delete (queued_at);

CREATE FUNCTION queue_attachment_file_delete() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO attachment_files_to_delete (path, dialog_id) VALUES (OLD.path, OLD.dialog_id);
    RETURN OLD;
END;
$$;

CREATE TRIGGER chat_attachments_queue_file_delete
    AFTER DELETE ON chat_attachments
    FOR EACH ROW EXECUTE FUNCTION queue_attachment_file_delete();
