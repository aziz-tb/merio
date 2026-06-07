-- name: get-prompt
SELECT id, created_at, updated_at, key, title, content FROM ai_prompts WHERE key = $1;

-- name: get-prompts
SELECT id, created_at, updated_at, key, title FROM ai_prompts ORDER BY title;
