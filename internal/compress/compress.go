package compress

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"log"
	"martnew/cmd/gophermart/initconf"
	"net/http"
	"strings"
)

const (
	BestCompression    = gzip.BestCompression
	BestSpeed          = gzip.BestSpeed
	DefaultCompression = gzip.DefaultCompression
	NoCompression      = gzip.NoCompression
)

var contentTypeToCompressMap = map[string]bool{
	"text/html":                true,
	"text/html; charset=utf-8": true,
	"application/json":         true,
}

type gzipWriter struct {
	gin.ResponseWriter
	writer *gzip.Writer
}

func (g gzipWriter) Write(data []byte) (int, error) {
	g.Header().Del("Content-Length")
	return g.writer.Write(data)
}

func (g *gzipWriter) WriteString(s string) (int, error) {
	g.Header().Del("Content-Length")
	return g.writer.Write([]byte(s))
}

func GzipResponseHandle(level int) gin.HandlerFunc {
	return func(c *gin.Context) {

		if !shouldCompress(c.Request) {
			c.Next()
			return
		}
		// создаём compress.Writer поверх текущего c.Writer
		gz, err := gzip.NewWriterLevel(c.Writer, level)
		if err != nil {
			io.WriteString(c.Writer, err.Error())
			return
		}
		c.Header("Content-Encoding", "compress")
		c.Header("Vary", "Accept-Encoding")
		c.Writer = &gzipWriter{c.Writer, gz}
		defer func() {
			c.Header("Content-Length", "0")
			gz.Close()
		}()
		c.Next()
	}
}

func shouldCompress(req *http.Request) bool {
	if !strings.Contains(req.Header.Get("Accept-Encoding"), "compress") {
		log.Println("There is no Accept-Encoding.")
		return false
	}

	// Если Content-Type запроса содержится в contentTypeToCompressMap -- включается сжатие
	if contentTypeToCompressMap[req.Header.Get("content-type")] {
		log.Println("compress compression for Content-Type", req.Header.Get("content-type"), "enabled")
		return true
	}

	if contentTypeToCompressMap[req.Header.Get("Accept")] {
		log.Println("compress compression for Content-Type", req.Header.Get("content-type"), "enabled")
		return true
	}

	log.Println("Default - do not encode. Content-Type is", req.Header.Get("Content-Type"))
	return false
}

func checkSign(body []byte, hash string, config *initconf.Config) (bool, error) {
	if config.Key == "" {
		return false, nil
	}
	var (
		data []byte // Декодированный hash с подписью
		err  error
		sign []byte // HMAC-подпись от идентификатора
	)

	data, err = hex.DecodeString(hash)
	if err != nil {
		log.Println("checkSign: hex.DecodeString error", err)
		return true, err
	}
	h := hmac.New(sha256.New, []byte(config.Key))
	h.Write(body)
	sign = h.Sum(nil)

	if hmac.Equal(sign, data) {
		fmt.Println("Подпись подлинна.")
		return true, nil
	} else {
		log.Println("Подпись неверна.")
		return true, fmt.Errorf("%s %v", "checkSign error: signature is incorrect ", err)
	}
}

func GzipRequestHandle(_ context.Context, config *initconf.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body []byte
		var newBody *bytes.Reader
		var gz *gzip.Reader
		var err error
		if c.Request.Header.Get(`Content-Encoding`) == `compress` {
			log.Println("c.Request.Header.Get(\"HashSHA256\") is :", c.Request.Header.Get("HashSHA256"))
			if hash := c.Request.Header.Get("HashSHA256"); hash != "" {
				body, err = io.ReadAll(c.Request.Body)
				if err != nil {
					log.Println("GzipRequestHandle: ioutil.ReadAll body error", err)
					c.Status(http.StatusInternalServerError)
					return
				}
				keyBool, err := checkSign(body, hash, config)
				if keyBool {
					if err != nil {
						log.Println("GzipRequestHandle: checkSign error", err)
						c.Status(http.StatusBadRequest)
						return
					}
				}
				// Ввиду того, что c.Request.Body был вычитан s body с помощью io.ReadAll -- делаем его копию для передачи в gz
				newBody = bytes.NewReader(body)
				gz, err = gzip.NewReader(newBody)
				if err != nil {
					log.Println("Error in GzipRequestHandle:", err)
					c.Status(http.StatusInternalServerError)
					return
				}
			} else {
				gz, err = gzip.NewReader(c.Request.Body)
				if err != nil {
					log.Println("Error in GzipRequestHandle:", err)
					c.Status(http.StatusInternalServerError)
					return
				}
			}
			log.Println("compress decompression")

			defer gz.Close()
			c.Request.Body = gz
			c.Next()
		}
	}
}
