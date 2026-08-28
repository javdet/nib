# Component Selection 
Backend + Worker: Go Concurrency
Frontend: React + TypeScript (Bun for build) Interactive dashboards, skill editor, plan visualization 
Database: PostgreSQL 
Vector storeL (KB) pgvector extension 
Message Queue: NATS 
Object Storage: S3 (cloud) Artifacts, IaC repos cache, logs 
MCP Runtime: Go SDK, MCP sessions managed by Backend 
LLM Abstraction: Thin Go layer with interface: Chat Completion (messages, tools) Anthropic/OpenAI/Ollama behind a single interface
