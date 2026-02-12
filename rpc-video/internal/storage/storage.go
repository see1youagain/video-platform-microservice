package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

var (
     StoragePath string
     ChunkSize   int64 = 2 * 1024 * 1024 // 默认 2MB
)

// InitStorage 初始化存储配置
func InitStorage() error {
     StoragePath = os.Getenv("STORAGE_PATH")
     if StoragePath == "" {
          StoragePath = "/tmp/video-platform"
     }

     chunkSizeStr := os.Getenv("CHUNK_SIZE")
     if chunkSizeStr != "" {
          size, err := strconv.ParseInt(chunkSizeStr, 10, 64)
          if err == nil {
               ChunkSize = size
          }
     }

     // 创建存储目录
     dirs := []string{
          filepath.Join(StoragePath, "chunks"),  // 分片临时目录
          filepath.Join(StoragePath, "files"),   // 最终文件目录
     }

     for _, dir := range dirs {
          if err := os.MkdirAll(dir, 0755); err != nil {
               return fmt.Errorf("failed to create directory %s: %w", dir, err)
          }
     }

     fmt.Printf("✅ 存储目录初始化成功: %s\n", StoragePath)
     return nil
}

// GetChunkPath 获取分片文件路径
func GetChunkPath(fileHash string, chunkIndex string) string {
     return filepath.Join(StoragePath, "chunks", fmt.Sprintf("%s_%s", fileHash, chunkIndex))
}

// GetFilePath 获取最终文件路径
func GetFilePath(fileHash string, filename string) string {
     ext := filepath.Ext(filename)
     return filepath.Join(StoragePath, "files", fileHash+ext)
}

// SaveChunk 保存分片数据
func SaveChunk(fileHash string, chunkIndex string, data []byte) error {
     chunkPath := GetChunkPath(fileHash, chunkIndex)
     
     // 确保目录存在
     if err := os.MkdirAll(filepath.Dir(chunkPath), 0755); err != nil {
          return fmt.Errorf("failed to create chunk directory: %w", err)
     }

     // 写入文件
     if err := os.WriteFile(chunkPath, data, 0644); err != nil {
          return fmt.Errorf("failed to write chunk: %w", err)
     }

     return nil
}

// ChunkExists 检查分片是否已存在
func ChunkExists(fileHash string, chunkIndex string) bool {
     chunkPath := GetChunkPath(fileHash, chunkIndex)
     _, err := os.Stat(chunkPath)
     return err == nil
}

// FileExists 检查文件是否已存在
func FileExists(fileHash string, filename string) bool {
     filePath := GetFilePath(fileHash, filename)
     _, err := os.Stat(filePath)
     return err == nil
}

// MergeChunks 合并分片文件
func MergeChunks(fileHash string, filename string, totalChunks int) error {
     finalPath := GetFilePath(fileHash, filename)

     // 创建最终文件
     finalFile, err := os.Create(finalPath)
     if err != nil {
          return fmt.Errorf("failed to create final file: %w", err)
     }
     defer finalFile.Close()

     // 按顺序合并分片
     for i := 0; i < totalChunks; i++ {
          chunkPath := GetChunkPath(fileHash, strconv.Itoa(i))
          
          chunkFile, err := os.Open(chunkPath)
          if err != nil {
               return fmt.Errorf("failed to open chunk %d: %w", i, err)
          }

          if _, err := io.Copy(finalFile, chunkFile); err != nil {
               chunkFile.Close()
               return fmt.Errorf("failed to copy chunk %d: %w", i, err)
          }
          chunkFile.Close()
     }

     // 删除临时分片文件
     for i := 0; i < totalChunks; i++ {
          chunkPath := GetChunkPath(fileHash, strconv.Itoa(i))
          os.Remove(chunkPath)
     }

     fmt.Printf("✅ 文件合并成功: %s\n", finalPath)
     return nil
}

// GetFileURL 获取文件访问 URL
func GetFileURL(fileHash string, filename string) string {
     // 这里返回一个简单的路径，实际生产环境可能需要返回 CDN URL
     return fmt.Sprintf("/files/%s%s", fileHash, filepath.Ext(filename))
}

// DeleteChunks 删除指定文件的所有分片
func DeleteChunks(fileHash string, totalChunks int) error {
     for i := 0; i < totalChunks; i++ {
          chunkPath := GetChunkPath(fileHash, strconv.Itoa(i))
          if err := os.Remove(chunkPath); err != nil && !os.IsNotExist(err) {
               return fmt.Errorf("failed to delete chunk %d: %w", i, err)
          }
     }
     return nil
}
