package middleware

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strings"

	"github.com/Mikimiya/remnawave-node/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
)

// Decompress is a middleware that decompresses gzip or zstd-encoded request bodies
func Decompress(log *logger.Logger) gin.HandlerFunc {
	// Create zstd decoder
	zstdDecoder, err := zstd.NewReader(nil)
	if err != nil {
		log.Errorw("Failed to create zstd decoder", "error", err)
	}

	return func(c *gin.Context) {
		if c.Request.Body == nil {
			c.Next()
			return
		}

		// Read the body to check for compression magic bytes
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.Next()
			return
		}
		c.Request.Body.Close()

		// Restore body immediately in case we don't decompress
		c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		// Check for compression
		contentEncoding := strings.ToLower(c.GetHeader("Content-Encoding"))
		isGzip := strings.Contains(contentEncoding, "gzip")
		isZstd := strings.Contains(contentEncoding, "zstd")

		// Debug logging
		if len(bodyBytes) > 0 {
			peek := bodyBytes
			if len(peek) > 10 {
				peek = peek[:10]
			}
			log.Debugw("Compression check",
				"encoding", contentEncoding,
				"magic_hex", fmt.Sprintf("%x", peek),
				"len", len(bodyBytes),
			)
		}

		// Magic bytes detection fallback
		if !isGzip && !isZstd && len(bodyBytes) >= 4 {
			// GZIP magic: 0x1f 0x8b
			if bodyBytes[0] == 0x1f && bodyBytes[1] == 0x8b {
				isGzip = true
			}
			// ZSTD magic: 0x28 0xb5 0x2f 0xfd
			if bodyBytes[0] == 0x28 && bodyBytes[1] == 0xb5 && bodyBytes[2] == 0x2f && bodyBytes[3] == 0xfd {
				isZstd = true
			}
		}

		var decompressedBody []byte

		if isGzip {
			log.Infow("Decompressing GZIP body")
			gzipReader, err := gzip.NewReader(bytes.NewReader(bodyBytes))
			if err == nil {
				defer gzipReader.Close()
				decompressedBody, err = io.ReadAll(gzipReader)
				if err != nil {
					log.Errorw("GZIP decompression failed", "error", err)
					// Fallback to original body if decompression fails
				}
			}
		} else if isZstd && zstdDecoder != nil {
			log.Infow("Decompressing ZSTD body")
			decompressedBody, err = zstdDecoder.DecodeAll(bodyBytes, nil)
			if err != nil {
				log.Errorw("ZSTD decompression failed", "error", err)
				// Fallback to original body if decompression fails
			}
		}

		if decompressedBody != nil {
			log.Infow("Decompression successful",
				"original_size", len(bodyBytes),
				"decompressed_size", len(decompressedBody))

			// Replace request body with decompressed data
			c.Request.Body = io.NopCloser(bytes.NewReader(decompressedBody))
			c.Request.ContentLength = int64(len(decompressedBody))
			c.Request.Header.Del("Content-Encoding")
		}

		c.Next()
	}
}
