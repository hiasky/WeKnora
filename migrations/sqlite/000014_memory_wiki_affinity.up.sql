CREATE TABLE IF NOT EXISTS memory_wiki_affinity (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    subject_id VARCHAR(512) NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    slug VARCHAR(512) NOT NULL,
    title VARCHAR(512) NOT NULL DEFAULT '',
    hits INTEGER NOT NULL DEFAULT 0,
    last_used_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_mem_wiki_affinity_scope
    ON memory_wiki_affinity (tenant_id, subject_id, knowledge_base_id, slug);

CREATE INDEX IF NOT EXISTS idx_mem_wiki_affinity_kb
    ON memory_wiki_affinity (tenant_id, subject_id, knowledge_base_id);
