RAG Workflow:

User Query
  ↓
Check Redis cache (optional)
  ↓
Embed query (small model)
  ↓
Search Qdrant for top-k matches
  ↓
Construct prompt with context
  ↓
Generate answer via OpenRouter LLM (cheap model like LLaMA3-70B)
  ↓
Cache + return response

Optimization Notes:

Use text-embedding-3-small (~$0.02/1K tokens) or bge-small-en

Use Groq or Mistral for fast, cheap inference

Chunk docs into ~300 token blocks, embed once, store in Qdrant

Use cosine similarity or HNSW + metadata filtering

Tools to Add Later:

Valkey	Caching answers	Free (self-hosted)
Promtail + Loki	Logging	Free (dev stack)
Rate Limiter (e.g. tollbooth)	API budget control	Free