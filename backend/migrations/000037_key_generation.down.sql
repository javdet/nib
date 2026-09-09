ALTER TABLE kb_chunks    ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE chat_dialogs ALTER COLUMN id SET DEFAULT gen_random_uuid();

DO $$
DECLARE
    next_id bigint;
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_attribute
        WHERE attrelid = 'chat_dialog_messages'::regclass
          AND attname = 'id' AND attidentity <> ''
    ) THEN
        SELECT COALESCE(max(id), 0) + 1 INTO next_id FROM chat_dialog_messages;

        ALTER TABLE chat_dialog_messages ALTER COLUMN id DROP IDENTITY;
        EXECUTE format('CREATE SEQUENCE chat_dialog_messages_id_seq START WITH %s OWNED BY chat_dialog_messages.id', next_id);
        ALTER TABLE chat_dialog_messages
            ALTER COLUMN id SET DEFAULT nextval('chat_dialog_messages_id_seq');
    END IF;
END
$$;
