package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/javdet/nib/internal/config"
	"github.com/javdet/nib/internal/kb"
	"github.com/javdet/nib/internal/kbdoc"
	"github.com/javdet/nib/internal/llm"
	"github.com/jackc/pgx/v5"
)

var (
	ErrConnectionURIRequired = errors.New("connectionUri is required")
	ErrFilenameRequired      = errors.New("filename is required")
	ErrEmptyFile             = errors.New("file is empty")
	ErrNoChunks              = errors.New("no chunks produced from document")
	ErrInvalidCollectionName = errors.New("invalid collection name")
)

const (
	knowledgeDefaultMetric  = "cosine"
	knowledgeEmbedBatchSize = 64
)

// KnowledgeConnection is the config-backed pgvector connection URI.
type KnowledgeConnection struct {
	ConnectionURI string `json:"connectionUri"`
}

// KnowledgeUploadResult summarizes a single-file ingest.
type KnowledgeUploadResult struct {
	Filename   string `json:"filename"`
	ChunkCount int    `json:"chunkCount"`
	Collection string `json:"collection"`
}

// KnowledgeStatus is the ingest state for a named collection.
type KnowledgeStatus struct {
	CollectionName string `json:"collectionName"`
	ConnectionURI  string `json:"connectionUri"`
	ChunkCount     int    `json:"chunkCount"`
	SourceURI      string `json:"sourceUri,omitempty"`
	Dimensions     int    `json:"dimensions,omitempty"`
	EmbeddingModel string `json:"embeddingModel,omitempty"`
}

// KnowledgeCollection summarizes a collection for listing.
type KnowledgeCollection struct {
	Name           string `json:"name"`
	ChunkCount     int    `json:"chunkCount"`
	SourceURI      string `json:"sourceUri,omitempty"`
	Dimensions     int    `json:"dimensions,omitempty"`
	EmbeddingModel string `json:"embeddingModel,omitempty"`
}

// KnowledgeService manages config-backed KB connection settings and document ingest.
type KnowledgeService struct {
	configPath        string
	defaultCollection string
	embeddingModel    string
	store             *kb.Store
	embedder          llm.Embedder
	docs              *kbdoc.Service
}

// NewKnowledgeService wires config I/O, the KB store, and the embedding provider.
func NewKnowledgeService(
	configPath string,
	defaultCollection string,
	embeddingModel string,
	store *kb.Store,
	embedder llm.Embedder,
	docs *kbdoc.Service,
) *KnowledgeService {
	if strings.TrimSpace(defaultCollection) == "" {
		defaultCollection = kb.DefaultCollectionName
	}
	if strings.TrimSpace(embeddingModel) == "" {
		embeddingModel = "text-embedding-3-small"
	}
	return &KnowledgeService{
		configPath:        configPath,
		defaultCollection: defaultCollection,
		embeddingModel:    embeddingModel,
		store:             store,
		embedder:          embedder,
		docs:              docs,
	}
}

func (s *KnowledgeService) resolveCollection(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return s.defaultCollection, nil
	}
	// kbdoc owns the pattern: a collection is also a document basename.
	if !kbdoc.ValidName(name) {
		return "", ErrInvalidCollectionName
	}
	return name, nil
}

// GetConnection reads knowledge_base.uri from the runtime config file.
func (s *KnowledgeService) GetConnection(_ context.Context) (KnowledgeConnection, error) {
	uri, err := config.ReadKnowledgeBaseURI(s.configPath)
	if err != nil {
		return KnowledgeConnection{}, fmt.Errorf("read knowledge base uri: %w", err)
	}
	return KnowledgeConnection{ConnectionURI: uri}, nil
}

// SetConnection updates knowledge_base.uri in the config file.
func (s *KnowledgeService) SetConnection(_ context.Context, uri string) (KnowledgeConnection, error) {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return KnowledgeConnection{}, ErrConnectionURIRequired
	}
	if err := config.WriteKnowledgeBaseURI(s.configPath, uri); err != nil {
		return KnowledgeConnection{}, fmt.Errorf("write knowledge base uri: %w", err)
	}
	return KnowledgeConnection{ConnectionURI: uri}, nil
}

// UploadDocument chunks content, embeds it, and replaces all chunks in the named collection.
func (s *KnowledgeService) UploadDocument(ctx context.Context, collection, filename string, content []byte) (KnowledgeUploadResult, error) {
	collectionName, err := s.resolveCollection(collection)
	if err != nil {
		return KnowledgeUploadResult{}, err
	}

	filename = strings.TrimSpace(filename)
	if filename == "" {
		return KnowledgeUploadResult{}, ErrFilenameRequired
	}
	text := string(content)
	if strings.TrimSpace(text) == "" {
		return KnowledgeUploadResult{}, ErrEmptyFile
	}

	pieces := kb.Chunk(text, kb.DefaultChunkSize, kb.DefaultChunkOverlap)
	if len(pieces) == 0 {
		return KnowledgeUploadResult{}, ErrNoChunks
	}

	embeddings, dim, err := s.embedTexts(ctx, pieces)
	if err != nil {
		return KnowledgeUploadResult{}, err
	}

	coll, err := s.store.UpsertCollection(ctx, collectionName, dim, s.embeddingModel, knowledgeDefaultMetric)
	if err != nil {
		return KnowledgeUploadResult{}, fmt.Errorf("upsert collection: %w", err)
	}

	inputs := make([]kb.ChunkInput, len(pieces))
	for i, piece := range pieces {
		inputs[i] = kb.ChunkInput{
			SourceURI:  filename,
			ChunkIndex: i,
			Content:    piece,
			Embedding:  embeddings[i],
		}
	}

	if err := s.store.ReplaceDocument(ctx, coll.ID, inputs); err != nil {
		return KnowledgeUploadResult{}, fmt.Errorf("replace document: %w", err)
	}

	// The file mirrors what was indexed, so it is written only once the chunks
	// are committed: a failed ingest must not leave a document on disk claiming
	// to be searchable.
	if err := s.docs.Save(coll.Name, text); err != nil {
		return KnowledgeUploadResult{}, fmt.Errorf("save knowledge document: %w", err)
	}

	return KnowledgeUploadResult{
		Filename:   filename,
		ChunkCount: len(inputs),
		Collection: coll.Name,
	}, nil
}

func (s *KnowledgeService) embedTexts(ctx context.Context, texts []string) ([][]float32, int, error) {
	var all [][]float32
	dim := 0

	for start := 0; start < len(texts); start += knowledgeEmbedBatchSize {
		end := start + knowledgeEmbedBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[start:end]
		vecs, d, err := s.embedder.Embed(ctx, batch)
		if err != nil {
			return nil, 0, fmt.Errorf("embed: %w", err)
		}
		if dim == 0 {
			dim = d
		} else if d != dim {
			return nil, 0, fmt.Errorf("embedding dimension drift: got %d, want %d", d, dim)
		}
		if len(vecs) != len(batch) {
			return nil, 0, fmt.Errorf("embedder returned %d vectors, want %d", len(vecs), len(batch))
		}
		all = append(all, vecs...)
	}

	return all, dim, nil
}

// GetDocument returns the source document last uploaded to a collection, or the
// skeleton compiled into the binary when nothing has been uploaded yet.
func (s *KnowledgeService) GetDocument(_ context.Context, collection string) (kbdoc.Document, error) {
	collectionName, err := s.resolveCollection(collection)
	if err != nil {
		return kbdoc.Document{}, err
	}
	return s.docs.Get(collectionName)
}

// Status returns collection metadata and chunk count for the named collection.
func (s *KnowledgeService) Status(ctx context.Context, collection string) (KnowledgeStatus, error) {
	collectionName, err := s.resolveCollection(collection)
	if err != nil {
		return KnowledgeStatus{}, err
	}

	uri, err := config.ReadKnowledgeBaseURI(s.configPath)
	if err != nil {
		return KnowledgeStatus{}, fmt.Errorf("read knowledge base uri: %w", err)
	}

	st, err := s.store.Status(ctx, collectionName)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return KnowledgeStatus{}, fmt.Errorf("kb status: %w", err)
	}

	out := KnowledgeStatus{
		CollectionName: collectionName,
		ConnectionURI:  uri,
		ChunkCount:     st.ChunkCount,
		SourceURI:      st.SourceURI,
	}
	if st.Collection.Dimensions > 0 {
		out.Dimensions = st.Collection.Dimensions
		out.EmbeddingModel = st.Collection.EmbeddingModel
	}
	return out, nil
}

// ListCollections returns all collections with ingest state.
func (s *KnowledgeService) ListCollections(ctx context.Context) ([]KnowledgeCollection, error) {
	summaries, err := s.store.ListCollections(ctx)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}

	out := make([]KnowledgeCollection, 0, len(summaries)+1)
	hasDefault := false
	for _, sum := range summaries {
		if sum.Name == s.defaultCollection {
			hasDefault = true
		}
		item := KnowledgeCollection{
			Name:       sum.Name,
			ChunkCount: sum.ChunkCount,
			SourceURI:  sum.SourceURI,
		}
		if sum.Dimensions > 0 {
			item.Dimensions = sum.Dimensions
			item.EmbeddingModel = sum.EmbeddingModel
		}
		out = append(out, item)
	}

	if !hasDefault {
		out = append([]KnowledgeCollection{{Name: s.defaultCollection}}, out...)
	}

	return out, nil
}

// ReadUploadContent reads all bytes from r, enforcing a maximum size.
func ReadUploadContent(r io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes < 1 {
		maxBytes = 32 << 20
	}
	limited := io.LimitReader(r, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read upload: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("file exceeds maximum size of %d bytes", maxBytes)
	}
	return data, nil
}
