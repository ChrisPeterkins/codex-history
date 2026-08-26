package data

const (
	// Preview text truncation limit (characters)
	maxPreviewLen = 120

	// Scanner buffer sizes for JSONL parsing
	scannerInitBuf  = 64 * 1024        // 64KB initial buffer
	scannerMaxBuf   = 1024 * 1024      // 1MB max for small files
	scannerLargeBuf = 10 * 1024 * 1024 // 10MB max for session files
)
