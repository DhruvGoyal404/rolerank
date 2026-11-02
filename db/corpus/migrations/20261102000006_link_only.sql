-- migrate:up
-- Greenhouse and Lever postings are shown as a link only (docs/SOURCES.md), so no excerpt is
-- kept for them.
UPDATE postings p SET excerpt = ''
FROM sources s
WHERE s.id = p.source_id AND s.name IN ('greenhouse', 'lever') AND p.excerpt <> '';

-- migrate:down
