-- chat_dialogs.categories is force-lowercased on write (service.normalizeTagList),
-- but tool_categories.name was stored verbatim and matched with cat.name = $1.
-- A category created as "Kubernetes" could therefore never be matched from a
-- dialog: the lookup returned zero rows, which is not an error, so the stage
-- subagent ran with an empty allow-set and no warning was logged.
--
-- Lowercase is the canonical form because that is the side that already
-- normalizes and the side that cannot be changed without rewriting every dialog.

-- Case-variant duplicates must be merged before the names collide. The lowest
-- id wins so the choice is deterministic; children are repointed at it first.
WITH survivor AS (
    SELECT lower(name) AS key, min(id::text)::uuid AS keep_id
    FROM tool_categories
    GROUP BY lower(name)
    HAVING count(*) > 1
),
losers AS (
    SELECT c.id AS loser_id, s.keep_id
    FROM tool_categories c
    JOIN survivor s ON s.key = lower(c.name) AND c.id <> s.keep_id
),
moved_patterns AS (
    UPDATE tool_category_patterns p
    SET category_id = l.keep_id
    FROM losers l
    WHERE p.category_id = l.loser_id
      AND NOT EXISTS (
          SELECT 1 FROM tool_category_patterns q
          WHERE q.category_id = l.keep_id AND q.pattern = p.pattern
      )
    RETURNING 1
),
moved_tools AS (
    UPDATE mcp_tool_categories tc
    SET category_id = l.keep_id
    FROM losers l
    WHERE tc.category_id = l.loser_id
      AND NOT EXISTS (
          SELECT 1 FROM mcp_tool_categories u
          WHERE u.category_id = l.keep_id AND u.tool_id = tc.tool_id
      )
    RETURNING 1
)
DELETE FROM tool_categories c USING losers l WHERE c.id = l.loser_id;

UPDATE tool_categories SET name = lower(name) WHERE name <> lower(name);

-- Redundant with UNIQUE (name) now that the values are canonical, but it is what
-- keeps them canonical: a direct INSERT of "Kubernetes" is rejected instead of
-- creating a second, unreachable category.
CREATE UNIQUE INDEX tool_categories_name_lower_key ON tool_categories (lower(name));
