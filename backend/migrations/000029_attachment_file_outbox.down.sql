DROP TRIGGER IF EXISTS chat_attachments_queue_file_delete ON chat_attachments;
DROP FUNCTION IF EXISTS queue_attachment_file_delete();
DROP TABLE IF EXISTS attachment_files_to_delete;
