package ufsdk

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

const (
	blkSIZE = 2 << 21
)

//Config 配置文件序列化所需的全部字段
type Config struct {
	PublicKey   string `json:"public_key"`
	PrivateKey  string `json:"private_key"`
	BucketName  string `json:"bucket_name"`
	UFileHost   string `json:"ufile_host"`
	UBucketHost string `json:"ubucket_host"`
}

//LoadConfig 从配置文件加载一个配置。
func LoadConfig(jsonPath string) (*Config, error) {
	file, err := openFile(jsonPath)
	if err != nil {
		return nil, err
	}
	configBytes, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}
	c := new(Config)
	err = json.Unmarshal(configBytes, c)
	if err != nil {
		return nil, err
	}
	return c, nil
}

//VerifyHTTPCode 检查 HTTP 的返回值是否为 2XX，如果不是就返回 false。
func VerifyHTTPCode(code int) bool {
	if code < http.StatusOK || code > http.StatusIMUsed {
		return false
	}
	return true
}

func openFile(path string) (*os.File, error) {
	return os.Open(path)
}

//getFileSize get opened file size
func getFileSize(f *os.File) int64 {
	fi, err := f.Stat()
	if err != nil {
		panic(err.Error())
	}
	return fi.Size()
}

func getMimeType(f *os.File) string {
	buffer := make([]byte, 512)

	_, err := f.Read(buffer)
	defer func() { f.Seek(0, 0) }() //revert file's seek
	if err != nil {
		return "plain/text"
	}

	return http.DetectContentType(buffer)
}

func calculateEtag(f *os.File) string {
	fsize := getFileSize(f)
	blkcnt := uint32(fsize / blkSIZE)
	if fsize%blkSIZE != 0 {
		blkcnt++
	}

	bs := make([]byte, 4)
	binary.LittleEndian.PutUint32(bs, blkcnt)

	h := sha1.New()
	buf := make([]byte, 0, 24)
	buf = append(buf, bs...)
	if fsize <= blkSIZE {
		io.Copy(h, f)
	} else {
		var i uint32
		for i = 0; i < blkcnt; i++ {
			shaBlk := sha1.New()
			io.Copy(shaBlk, io.LimitReader(f, blkSIZE))
			io.Copy(h, bytes.NewReader(shaBlk.Sum(nil)))
		}
	}
	buf = h.Sum(buf)
	etag := base64.URLEncoding.EncodeToString(buf)
	return etag
}

func makeBoundry() string {
	h := md5.New()
	t := time.Now()
	io.WriteString(h, t.String())
	return fmt.Sprintf("%x", h.Sum(nil))
}

func makeFormBody(authorization, boundry, keyName, mimeType string, file *os.File) *bytes.Buffer {
	boundry = "--" + boundry + "\r\n"
	boundryBytes := []byte(boundry)
	body := new(bytes.Buffer)

	body.Write(boundryBytes)
	body.Write(makeFormField("Authorization", authorization))
	body.Write(boundryBytes)
	body.Write(makeFormField("Content-Type", mimeType))
	body.Write(boundryBytes)
	body.Write(makeFormField("FileName", keyName))
	body.Write(boundryBytes)

	h := md5.New()
	io.Copy(h, file)
	md5Str := fmt.Sprintf("%x", h.Sum(nil))
	body.Write(makeFormField("Content-MD5", md5Str))
	body.Write(boundryBytes)

	addtionalStr := fmt.Sprintf("Content-Disposition: form-data; name=\"file\"; filename=\"%s\"\r\n", keyName)
	addtionalStr += fmt.Sprintf("Content-Type: %s\r\n\r\n", mimeType)
	body.Write([]byte(addtionalStr))
	body.ReadFrom(file)
	body.Write([]byte("\r\n"))
	body.Write(boundryBytes)

	return body
}

func makeFormField(key, value string) []byte {
	keyStr := fmt.Sprintf("Content-Disposition: form-data; name=\"%s\"\r\n\r\n", key)
	valueStr := fmt.Sprintf("%s\r\n", value)
	return []byte(keyStr + valueStr)
}
