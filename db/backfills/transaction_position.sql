DO $$
BEGIN
    IF to_regclass('public.entries') IS NOT NULL THEN
        ALTER TABLE public.entries
            ADD COLUMN IF NOT EXISTS transaction_position bigint;

        WITH ranked_entries AS (
            SELECT
                id,
                row_number() OVER (
                    PARTITION BY transaction_id
                    ORDER BY account_id, sequence_number, id
                ) - 1 AS position
            FROM public.entries
            WHERE transaction_position IS NULL
        )
        UPDATE public.entries AS entry
        SET transaction_position = ranked_entries.position
        FROM ranked_entries
        WHERE entry.id = ranked_entries.id;
    END IF;
END
$$;
