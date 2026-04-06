//go:build integration

package cli

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/briandeitte/photo-copy/internal/testutil/mockserver"
)

// flickrMultiSizesResponse builds a Flickr getSizes JSON response with multiple sizes.
func flickrMultiSizesResponse(sizes []map[string]string) map[string]any {
	return map[string]any{
		"sizes": map[string]any{
			"size": sizes,
		},
		"stat": "ok",
	}
}

// buildMinimalJPEG constructs a minimal valid JPEG file in memory.
func buildMinimalJPEG() []byte {
	soi := []byte{0xFF, 0xD8}
	app0Payload := []byte("JFIF\x00\x01\x01\x00\x00\x01\x00\x01\x00\x00")
	app0Len := uint16(len(app0Payload) + 2)
	app0 := []byte{0xFF, 0xE0, byte(app0Len >> 8), byte(app0Len)}
	app0 = append(app0, app0Payload...)
	sos := []byte{0xFF, 0xDA, 0x00, 0x0C, 0x03, 0x01, 0x00, 0x02, 0x11, 0x03, 0x11, 0x00, 0x3F, 0x00}
	eoi := []byte{0xFF, 0xD9}

	var data []byte
	data = append(data, soi...)
	data = append(data, app0...)
	data = append(data, sos...)
	data = append(data, eoi...)
	return data
}

// readXMPFromJPEG extracts XMP content from a JPEG file's APP1 segment.
func readXMPFromJPEG(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const xmpNS = "http://ns.adobe.com/xap/1.0/\x00"
	i := 2 // skip SOI
	for i+4 <= len(data) {
		if data[i] != 0xFF {
			break
		}
		marker := data[i+1]
		if marker == 0xD9 || marker == 0xDA {
			break
		}
		segLen := int(data[i+2])<<8 | int(data[i+3])
		payload := data[i+4 : i+2+segLen]
		if marker == 0xE1 && strings.HasPrefix(string(payload), xmpNS) {
			return string(payload[len(xmpNS):])
		}
		i += 2 + segLen
	}
	return ""
}

// buildMinimalMP4 constructs a minimal valid MP4 file with ftyp + moov(mvhd) structure.
// This is sufficient for mp4meta.SetCreationTime and mp4meta.SetXMPMetadata to operate on.
func buildMinimalMP4() []byte {
	var buf bytes.Buffer

	// ftyp box (20 bytes): size(4) + "ftyp"(4) + major_brand(4) + minor_version(4) + compatible(4)
	ftypPayload := []byte{
		'i', 's', 'o', 'm', // major brand
		0x00, 0x00, 0x02, 0x00, // minor version
		'i', 's', 'o', 'm', // compatible brand
	}
	binary.Write(&buf, binary.BigEndian, uint32(8+len(ftypPayload))) //nolint:errcheck
	buf.WriteString("ftyp")
	buf.Write(ftypPayload)

	// Build mvhd box (V0: 108 bytes payload)
	// Version 0 uses 32-bit timestamps
	mvhdPayload := make([]byte, 108)
	// version=0, flags=0 (first 4 bytes, already zero)
	// creation_time at offset 4 (32-bit, zero)
	// modification_time at offset 8 (32-bit, zero)
	// timescale at offset 12
	binary.BigEndian.PutUint32(mvhdPayload[12:16], 1000) // timescale
	// duration at offset 16 (32-bit, zero)
	// rate at offset 20
	binary.BigEndian.PutUint32(mvhdPayload[20:24], 0x00010000) // rate = 1.0
	// volume at offset 24
	binary.BigEndian.PutUint16(mvhdPayload[24:26], 0x0100) // volume = 1.0
	// matrix at offset 36 (9 * 4 bytes) — identity matrix
	binary.BigEndian.PutUint32(mvhdPayload[36:40], 0x00010000)
	binary.BigEndian.PutUint32(mvhdPayload[52:56], 0x00010000)
	binary.BigEndian.PutUint32(mvhdPayload[72:76], 0x40000000)
	// next_track_id at offset 104
	binary.BigEndian.PutUint32(mvhdPayload[104:108], 2)

	mvhdSize := uint32(8 + len(mvhdPayload))

	// Build tkhd box (V0: 92 bytes payload)
	tkhdPayload := make([]byte, 92)
	// version=0, flags=3 (track enabled + in movie)
	tkhdPayload[3] = 3
	// track_id at offset 12
	binary.BigEndian.PutUint32(tkhdPayload[12:16], 1)
	// matrix at offset 36
	binary.BigEndian.PutUint32(tkhdPayload[36:40], 0x00010000)
	binary.BigEndian.PutUint32(tkhdPayload[52:56], 0x00010000)
	binary.BigEndian.PutUint32(tkhdPayload[72:76], 0x40000000)

	tkhdSize := uint32(8 + len(tkhdPayload))

	// Build mdhd box (V0: 32 bytes payload)
	mdhdPayload := make([]byte, 32)
	// timescale at offset 12
	binary.BigEndian.PutUint32(mdhdPayload[12:16], 1000)

	mdhdSize := uint32(8 + len(mdhdPayload))

	// mdia = mdhd
	mdiaSize := uint32(8) + mdhdSize
	// trak = tkhd + mdia
	trakSize := uint32(8) + tkhdSize + mdiaSize
	// moov = mvhd + trak
	moovSize := uint32(8) + mvhdSize + trakSize

	// Write moov
	binary.Write(&buf, binary.BigEndian, moovSize) //nolint:errcheck
	buf.WriteString("moov")

	binary.Write(&buf, binary.BigEndian, mvhdSize) //nolint:errcheck
	buf.WriteString("mvhd")
	buf.Write(mvhdPayload)

	binary.Write(&buf, binary.BigEndian, trakSize) //nolint:errcheck
	buf.WriteString("trak")

	binary.Write(&buf, binary.BigEndian, tkhdSize) //nolint:errcheck
	buf.WriteString("tkhd")
	buf.Write(tkhdPayload)

	binary.Write(&buf, binary.BigEndian, mdiaSize) //nolint:errcheck
	buf.WriteString("mdia")

	binary.Write(&buf, binary.BigEndian, mdhdSize) //nolint:errcheck
	buf.WriteString("mdhd")
	buf.Write(mdhdPayload)

	return buf.Bytes()
}

// readXMPFromMP4 extracts XMP content from an MP4 file's UUID box.
func readXMPFromMP4(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	xmpUUID := []byte{0xBE, 0x7A, 0xCF, 0xCB, 0x97, 0xA9, 0x42, 0xE8, 0x9C, 0x71, 0x99, 0x94, 0x91, 0xE3, 0xAF, 0xAC}
	pos := 0
	for pos+8 <= len(data) {
		boxSize := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		boxType := string(data[pos+4 : pos+8])
		if boxSize == 0 {
			boxSize = len(data) - pos
		}
		if boxSize < 8 || pos+boxSize > len(data) {
			break
		}
		if boxType == "uuid" && pos+8+16 <= pos+boxSize {
			if bytes.Equal(data[pos+8:pos+8+16], xmpUUID) {
				return string(data[pos+8+16 : pos+boxSize])
			}
		}
		pos += boxSize
	}
	return ""
}

// --- Flickr Download Tests ---

func TestFlickrDownload_HappyPath(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	photos := []map[string]string{
		{"id": "1", "secret": "aaa", "server": "1", "title": "photo1"},
		{"id": "2", "secret": "bbb", "server": "1", "title": "photo2"},
		{"id": "3", "secret": "ccc", "server": "1", "title": "photo3"},
	}

	var mock *mockserver.FlickrMock
	mock = mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondJSON(200, flickrPhotosResponse(photos, 1, 1, 3))).
		OnGetSizes(func(w http.ResponseWriter, r *http.Request) {
			photoID := r.URL.Query().Get("photo_id")
			mockserver.RespondJSON(200, flickrSizesResponse(
				mock.Server.URL+"/download/"+photoID+".jpg",
			))(w, r)
		}).
		OnDownload(mockserver.RespondBytes(200, testImageData)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.APIURL)

	err := executeCmd(t, "flickr", "download", outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify 3 files downloaded
	for _, p := range photos {
		filename := fmt.Sprintf("%s_%s.jpg", p["id"], p["secret"])
		data, err := os.ReadFile(filepath.Join(outputDir, filename))
		if err != nil {
			t.Errorf("expected file %s: %v", filename, err)
			continue
		}
		if string(data) != string(testImageData) {
			t.Errorf("file %s content mismatch", filename)
		}
	}

	// Verify transfer log
	logLines := readLines(t, filepath.Join(outputDir, "transfer.log"))
	if len(logLines) != 3 {
		t.Errorf("transfer log has %d entries, want 3", len(logLines))
	}
}

func TestFlickrDownload_Pagination(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	page1Photos := []map[string]string{
		{"id": "1", "secret": "aaa", "server": "1", "title": "p1"},
		{"id": "2", "secret": "bbb", "server": "1", "title": "p2"},
	}
	page2Photos := []map[string]string{
		{"id": "3", "secret": "ccc", "server": "1", "title": "p3"},
	}

	var mock *mockserver.FlickrMock
	mock = mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondSequence(
			mockserver.RespondJSON(200, flickrPhotosResponse(page1Photos, 1, 2, 3)),
			mockserver.RespondJSON(200, flickrPhotosResponse(page2Photos, 2, 2, 3)),
		)).
		OnGetSizes(func(w http.ResponseWriter, r *http.Request) {
			photoID := r.URL.Query().Get("photo_id")
			mockserver.RespondJSON(200, flickrSizesResponse(
				mock.Server.URL+"/download/"+photoID+".jpg",
			))(w, r)
		}).
		OnDownload(mockserver.RespondBytes(200, testImageData)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.APIURL)

	err := executeCmd(t, "flickr", "download", outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify all 3 files from 2 pages
	for _, id := range []string{"1_aaa", "2_bbb", "3_ccc"} {
		if _, err := os.Stat(filepath.Join(outputDir, id+".jpg")); err != nil {
			t.Errorf("missing file %s.jpg", id)
		}
	}
}

func TestFlickrDownload_ResumesFromLog(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	// Pre-populate transfer log with one file
	_ = os.WriteFile(filepath.Join(outputDir, "transfer.log"), []byte("1_aaa.jpg\n"), 0644)

	photos := []map[string]string{
		{"id": "1", "secret": "aaa", "server": "1", "title": "p1"},
		{"id": "2", "secret": "bbb", "server": "1", "title": "p2"},
	}

	var mock *mockserver.FlickrMock
	mock = mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondJSON(200, flickrPhotosResponse(photos, 1, 1, 2))).
		OnGetSizes(func(w http.ResponseWriter, r *http.Request) {
			photoID := r.URL.Query().Get("photo_id")
			if photoID == "1" {
				t.Error("getSizes should not be called for already-downloaded photo 1")
			}
			mockserver.RespondJSON(200, flickrSizesResponse(
				mock.Server.URL+"/download/"+photoID+".jpg",
			))(w, r)
		}).
		OnDownload(mockserver.RespondBytes(200, testImageData)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.APIURL)

	err := executeCmd(t, "flickr", "download", outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only photo 2 should be downloaded
	if _, err := os.Stat(filepath.Join(outputDir, "2_bbb.jpg")); err != nil {
		t.Error("photo 2 should have been downloaded")
	}
	// Photo 1 file should NOT exist (was in log but not re-downloaded)
	if _, err := os.Stat(filepath.Join(outputDir, "1_aaa.jpg")); err == nil {
		t.Error("photo 1 should not have been re-downloaded")
	}

	// Transfer log should now have both entries
	logLines := readLines(t, filepath.Join(outputDir, "transfer.log"))
	if len(logLines) != 2 {
		t.Errorf("transfer log has %d entries, want 2", len(logLines))
	}
}

func TestFlickrDownload_RetryOn429(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	photos := []map[string]string{
		{"id": "1", "secret": "aaa", "server": "1", "title": "p1"},
	}

	var mock *mockserver.FlickrMock
	mock = mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondJSON(200, flickrPhotosResponse(photos, 1, 1, 1))).
		OnGetSizes(mockserver.RespondSequence(
			mockserver.RespondStatus(429),
			func(w http.ResponseWriter, r *http.Request) {
				photoID := r.URL.Query().Get("photo_id")
				mockserver.RespondJSON(200, flickrSizesResponse(
					mock.Server.URL+"/download/"+photoID+".jpg",
				))(w, r)
			},
		)).
		OnDownload(mockserver.RespondBytes(200, testImageData)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.APIURL)

	err := executeCmd(t, "flickr", "download", outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outputDir, "1_aaa.jpg")); err != nil {
		t.Error("photo should have been downloaded after retry")
	}
}

func TestFlickrDownload_RetryOn5xx(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	photos := []map[string]string{
		{"id": "1", "secret": "aaa", "server": "1", "title": "p1"},
	}

	var mock *mockserver.FlickrMock
	mock = mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondJSON(200, flickrPhotosResponse(photos, 1, 1, 1))).
		OnGetSizes(func(w http.ResponseWriter, r *http.Request) {
			photoID := r.URL.Query().Get("photo_id")
			mockserver.RespondJSON(200, flickrSizesResponse(
				mock.Server.URL+"/download/"+photoID+".jpg",
			))(w, r)
		}).
		OnDownload(mockserver.RespondSequence(
			mockserver.RespondStatus(500),
			mockserver.RespondBytes(200, testImageData),
		)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.APIURL)

	err := executeCmd(t, "flickr", "download", outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outputDir, "1_aaa.jpg"))
	if err != nil {
		t.Fatal("photo should have been downloaded after 5xx retry")
	}
	if string(data) != string(testImageData) {
		t.Error("downloaded file content mismatch")
	}
}

func TestFlickrDownload_LimitFlag(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	photos := []map[string]string{
		{"id": "1", "secret": "aaa", "server": "1", "title": "p1"},
		{"id": "2", "secret": "bbb", "server": "1", "title": "p2"},
		{"id": "3", "secret": "ccc", "server": "1", "title": "p3"},
		{"id": "4", "secret": "ddd", "server": "1", "title": "p4"},
		{"id": "5", "secret": "eee", "server": "1", "title": "p5"},
	}

	var mock *mockserver.FlickrMock
	mock = mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondJSON(200, flickrPhotosResponse(photos, 1, 1, 5))).
		OnGetSizes(func(w http.ResponseWriter, r *http.Request) {
			photoID := r.URL.Query().Get("photo_id")
			mockserver.RespondJSON(200, flickrSizesResponse(
				mock.Server.URL+"/download/"+photoID+".jpg",
			))(w, r)
		}).
		OnDownload(mockserver.RespondBytes(200, testImageData)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.APIURL)

	err := executeCmd(t, "flickr", "download", "--limit", "2", outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Count downloaded files (excluding transfer.log and report files)
	entries, _ := os.ReadDir(outputDir)
	count := 0
	for _, e := range entries {
		if e.Name() != "transfer.log" && !strings.HasPrefix(e.Name(), "photo-copy-report-") {
			count++
		}
	}
	if count != 2 {
		t.Errorf("downloaded %d files, want 2", count)
	}
}

func TestFlickrDownload_RetryOnHTMLResponse(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	photos := []map[string]string{
		{"id": "1", "secret": "aaa", "server": "1", "title": "p1"},
	}

	var mock *mockserver.FlickrMock
	mock = mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondJSON(200, flickrPhotosResponse(photos, 1, 1, 1))).
		OnGetSizes(mockserver.RespondSequence(
			// First call returns HTML (simulating Flickr error page with 200 status)
			func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(200)
				_, _ = w.Write([]byte("<html><body>Error</body></html>"))
			},
			// Second call returns valid JSON
			func(w http.ResponseWriter, r *http.Request) {
				photoID := r.URL.Query().Get("photo_id")
				mockserver.RespondJSON(200, flickrSizesResponse(
					mock.Server.URL+"/download/"+photoID+".jpg",
				))(w, r)
			},
		)).
		OnDownload(mockserver.RespondBytes(200, testImageData)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.APIURL)

	err := executeCmd(t, "flickr", "download", outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outputDir, "1_aaa.jpg")); err != nil {
		t.Error("photo should have been downloaded after HTML retry")
	}
}

func TestFlickrDownload_VideoFallbackOn404(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	photos := []map[string]string{
		{"id": "1", "secret": "aaa", "server": "1", "title": "video1"},
	}

	var mock *mockserver.FlickrMock
	mock = mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondJSON(200, flickrPhotosResponse(photos, 1, 1, 1))).
		OnGetSizes(func(w http.ResponseWriter, r *http.Request) {
			// Return multiple sizes: Video Original (will 404) and Video Player (fallback)
			mockserver.RespondJSON(200, flickrMultiSizesResponse([]map[string]string{
				{"label": "Video Original", "source": mock.Server.URL + "/download/orig.mp4"},
				{"label": "Video Player", "source": mock.Server.URL + "/download/player.mp4"},
			}))(w, r)
		}).
		OnDownload(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "orig.mp4") {
				w.WriteHeader(404)
				return
			}
			// Fallback URL succeeds
			mockserver.RespondBytes(200, testImageData)(w, r)
		}).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.APIURL)

	err := executeCmd(t, "flickr", "download", outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// File should have been downloaded using the fallback URL
	if _, err := os.Stat(filepath.Join(outputDir, "1_aaa.mp4")); err != nil {
		t.Error("video should have been downloaded via fallback URL")
	}

	// Verify transfer log has the entry
	logLines := readLines(t, filepath.Join(outputDir, "transfer.log"))
	if len(logLines) != 1 {
		t.Errorf("transfer log has %d entries, want 1", len(logLines))
	}
}

func TestFlickrDownload_PreservesOriginalDates(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	photos := []map[string]string{
		{
			"id": "1", "secret": "aaa", "server": "1", "title": "photo1",
			"datetaken": "2020-06-15 14:30:00", "dateupload": "1592234567",
		},
	}

	var mock *mockserver.FlickrMock
	mock = mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondJSON(200, flickrPhotosResponse(photos, 1, 1, 1))).
		OnGetSizes(func(w http.ResponseWriter, r *http.Request) {
			photoID := r.URL.Query().Get("photo_id")
			mockserver.RespondJSON(200, flickrSizesResponse(
				mock.Server.URL+"/download/"+photoID+".jpg",
			))(w, r)
		}).
		OnDownload(mockserver.RespondBytes(200, testImageData)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.APIURL)

	err := executeCmd(t, "flickr", "download", outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file system timestamp was set to the date_taken value
	filePath := filepath.Join(outputDir, "1_aaa.jpg")
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("file not found: %v", err)
	}
	expectedTime := time.Date(2020, 6, 15, 14, 30, 0, 0, time.UTC)
	modTime := info.ModTime().UTC()
	if !modTime.Equal(expectedTime) {
		t.Errorf("file mod time = %v, want %v", modTime, expectedTime)
	}
}

func TestFlickrDownload_EmbedsXMPMetadata(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	// Build a minimal valid JPEG to serve as download content.
	jpegData := buildMinimalJPEG()

	photos := []map[string]any{
		{
			"id": "1", "secret": "aaa", "server": "1",
			"title":       "Sunset <Photo>",
			"datetaken":   "2020-06-15 14:30:00",
			"dateupload":  "1592234567",
			"description": map[string]string{"_content": "<b>A beautiful</b> sunset"},
			"tags":        "nature sunset sky",
		},
	}

	var mock *mockserver.FlickrMock
	mock = mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondJSON(200, map[string]any{
			"photos": map[string]any{
				"page":  1,
				"pages": 1,
				"total": 1,
				"photo": photos,
			},
			"stat": "ok",
		})).
		OnGetSizes(func(w http.ResponseWriter, r *http.Request) {
			photoID := r.URL.Query().Get("photo_id")
			mockserver.RespondJSON(200, flickrSizesResponse(
				mock.Server.URL+"/download/"+photoID+".jpg",
			))(w, r)
		}).
		OnDownload(mockserver.RespondBytes(200, jpegData)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.APIURL)

	err := executeCmd(t, "flickr", "download", outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read the downloaded file and verify XMP metadata was embedded.
	filePath := filepath.Join(outputDir, "1_aaa.jpg")
	xmp := readXMPFromJPEG(t, filePath)
	if xmp == "" {
		t.Fatal("no XMP metadata found in downloaded JPEG")
	}
	if !strings.Contains(xmp, "Sunset &lt;Photo&gt;") {
		t.Errorf("XMP should contain escaped title, got: %s", xmp)
	}
	if !strings.Contains(xmp, "A beautiful sunset") {
		t.Errorf("XMP should contain stripped description, got: %s", xmp)
	}
	if !strings.Contains(xmp, "<rdf:li>nature</rdf:li>") {
		t.Errorf("XMP should contain tags, got: %s", xmp)
	}
	if !strings.Contains(xmp, "<xmp:CreateDate>2020-06-15T14:30:00Z</xmp:CreateDate>") {
		t.Errorf("XMP should contain CreateDate from date_taken, got: %s", xmp)
	}
}

func TestFlickrDownload_XMPCreateDateFallsBackToDateUpload(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	jpegData := buildMinimalJPEG()

	// date_taken is the sentinel value that Flickr returns when the actual
	// capture date is unknown; resolvePhotoDate should fall back to date_upload.
	photos := []map[string]any{
		{
			"id": "1", "secret": "aaa", "server": "1",
			"title":       "Old Photo",
			"datetaken":   "1970-01-01 00:00:00",
			"dateupload":  "1592234567",
			"description": map[string]string{"_content": ""},
			"tags":        "",
		},
	}

	var mock *mockserver.FlickrMock
	mock = mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondJSON(200, map[string]any{
			"photos": map[string]any{
				"page":  1,
				"pages": 1,
				"total": 1,
				"photo": photos,
			},
			"stat": "ok",
		})).
		OnGetSizes(func(w http.ResponseWriter, r *http.Request) {
			photoID := r.URL.Query().Get("photo_id")
			mockserver.RespondJSON(200, flickrSizesResponse(
				mock.Server.URL+"/download/"+photoID+".jpg",
			))(w, r)
		}).
		OnDownload(mockserver.RespondBytes(200, jpegData)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.APIURL)

	err := executeCmd(t, "flickr", "download", outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	filePath := filepath.Join(outputDir, "1_aaa.jpg")
	xmp := readXMPFromJPEG(t, filePath)
	if xmp == "" {
		t.Fatal("no XMP metadata found in downloaded JPEG")
	}

	// date_upload 1592234567 = 2020-06-15T14:42:47Z
	expectedDate := time.Unix(1592234567, 0).UTC().Format("2006-01-02T15:04:05Z")
	want := "<xmp:CreateDate>" + expectedDate + "</xmp:CreateDate>"
	if !strings.Contains(xmp, want) {
		t.Errorf("XMP should contain CreateDate from date_upload fallback, want %s, got: %s", want, xmp)
	}
}

// --- Flickr Upload Tests ---

func TestFlickrUpload_HappyPath(t *testing.T) {
	inputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	// Create test media files
	_ = os.WriteFile(filepath.Join(inputDir, "photo1.jpg"), testImageData, 0644)
	_ = os.WriteFile(filepath.Join(inputDir, "photo2.jpg"), []byte("jpeg-data-2"), 0644)

	mock := mockserver.NewFlickr(t).
		OnUpload(mockserver.RespondStatus(200)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_UPLOAD_URL", mock.UploadURL)

	err := executeCmd(t, "flickr", "upload", inputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify 2 upload requests were made
	uploadRequests := 0
	for _, req := range mock.Requests() {
		if strings.HasPrefix(req.Path, "/services/upload/") {
			uploadRequests++
		}
	}
	if uploadRequests != 2 {
		t.Errorf("got %d upload requests, want 2", uploadRequests)
	}
}

func TestFlickrUpload_SkipsNonMedia(t *testing.T) {
	inputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	_ = os.WriteFile(filepath.Join(inputDir, "photo.jpg"), testImageData, 0644)
	_ = os.WriteFile(filepath.Join(inputDir, "readme.txt"), []byte("not media"), 0644)
	_ = os.WriteFile(filepath.Join(inputDir, "data.csv"), []byte("1,2,3"), 0644)

	mock := mockserver.NewFlickr(t).
		OnUpload(mockserver.RespondStatus(200)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_UPLOAD_URL", mock.UploadURL)

	err := executeCmd(t, "flickr", "upload", inputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	uploadRequests := 0
	for _, req := range mock.Requests() {
		if strings.HasPrefix(req.Path, "/services/upload/") {
			uploadRequests++
		}
	}
	if uploadRequests != 1 {
		t.Errorf("got %d upload requests, want 1 (only .jpg)", uploadRequests)
	}
}

func TestFlickrUpload_LimitFlag(t *testing.T) {
	inputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	_ = os.WriteFile(filepath.Join(inputDir, "a.jpg"), testImageData, 0644)
	_ = os.WriteFile(filepath.Join(inputDir, "b.jpg"), testImageData, 0644)
	_ = os.WriteFile(filepath.Join(inputDir, "c.jpg"), testImageData, 0644)

	mock := mockserver.NewFlickr(t).
		OnUpload(mockserver.RespondStatus(200)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_UPLOAD_URL", mock.UploadURL)

	err := executeCmd(t, "flickr", "upload", "--limit", "1", inputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	uploadRequests := 0
	for _, req := range mock.Requests() {
		if strings.HasPrefix(req.Path, "/services/upload/") {
			uploadRequests++
		}
	}
	if uploadRequests != 1 {
		t.Errorf("got %d upload requests, want 1", uploadRequests)
	}
}

func TestFlickrUpload_DateRange(t *testing.T) {
	inputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	// Create files with known modification times
	inRange := filepath.Join(inputDir, "in_range.jpg")
	outRange := filepath.Join(inputDir, "out_range.jpg")
	_ = os.WriteFile(inRange, testImageData, 0644)
	_ = os.WriteFile(outRange, testImageData, 0644)

	// Set mod times: in_range = 2022, out_range = 2019
	_ = os.Chtimes(inRange, time.Now(), time.Date(2022, 6, 15, 10, 0, 0, 0, time.Local))
	_ = os.Chtimes(outRange, time.Now(), time.Date(2019, 1, 1, 10, 0, 0, 0, time.Local))

	mock := mockserver.NewFlickr(t).
		OnUpload(mockserver.RespondStatus(200)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_UPLOAD_URL", mock.UploadURL)

	err := executeCmd(t, "flickr", "upload", "--date-range", "2021-01-01:2023-12-31", inputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only in_range.jpg should have been uploaded
	uploadRequests := 0
	for _, req := range mock.Requests() {
		if strings.HasPrefix(req.Path, "/services/upload/") {
			uploadRequests++
		}
	}
	if uploadRequests != 1 {
		t.Errorf("got %d upload requests, want 1 (only in_range.jpg)", uploadRequests)
	}
}

func TestFlickrUpload_FailsOnError(t *testing.T) {
	inputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	_ = os.WriteFile(filepath.Join(inputDir, "photo.jpg"), testImageData, 0644)

	mock := mockserver.NewFlickr(t).
		OnUpload(mockserver.RespondStatus(500)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_UPLOAD_URL", mock.UploadURL)

	err := executeCmd(t, "flickr", "upload", inputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Upload now continues past failures and records them in the result.
	// Verify that the report file was written and contains the failure.
	entries, _ := os.ReadDir(inputDir)
	var reportFound bool
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "photo-copy-report-") {
			reportFound = true
			data, _ := os.ReadFile(filepath.Join(inputDir, e.Name()))
			if !strings.Contains(string(data), "failed:    1") {
				t.Errorf("report should show 1 failed, got: %s", string(data))
			}
			if !strings.Contains(string(data), "upload failed HTTP 500") {
				t.Errorf("report should mention upload failure, got: %s", string(data))
			}
		}
	}
	if !reportFound {
		t.Error("expected a report file to be written")
	}
}

// --- Flag Integration Tests (Flickr) ---

func TestFlickrDownload_NoMetadata(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	photos := []map[string]any{
		{
			"id": "1", "secret": "aaa", "server": "1",
			"title":       "Test Photo",
			"datetaken":   "2020-06-15 10:30:00",
			"dateupload":  "1592217000",
			"description": map[string]string{"_content": "A test description"},
			"tags":        "tag1 tag2",
		},
	}

	// Build a minimal valid JPEG so metadata embedding would normally modify it.
	jpegData := buildMinimalJPEG()

	var mock *mockserver.FlickrMock
	mock = mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondJSON(200, map[string]any{
			"photos": map[string]any{
				"page":  1,
				"pages": 1,
				"total": 1,
				"photo": photos,
			},
			"stat": "ok",
		})).
		OnGetSizes(func(w http.ResponseWriter, r *http.Request) {
			photoID := r.URL.Query().Get("photo_id")
			mockserver.RespondJSON(200, flickrSizesResponse(
				mock.Server.URL+"/download/"+photoID+".jpg",
			))(w, r)
		}).
		OnDownload(mockserver.RespondBytes(200, jpegData)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.APIURL)

	err := executeCmd(t, "flickr", "download", "--no-metadata", outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file was downloaded
	filePath := filepath.Join(outputDir, "1_aaa.jpg")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("expected file to be downloaded: %v", err)
	}

	// With --no-metadata, content should be exactly what the server sent (no XMP injected)
	if !bytes.Equal(data, jpegData) {
		t.Errorf("file content should be unchanged with --no-metadata; got %d bytes, want %d bytes", len(data), len(jpegData))
	}
}

func TestFlickrDownload_DateRange(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	photos := []map[string]string{
		{"id": "1", "secret": "aaa", "server": "1", "title": "old", "datetaken": "2020-06-15 10:30:00", "dateupload": "1592217000"},
		{"id": "2", "secret": "bbb", "server": "1", "title": "mid", "datetaken": "2022-06-15 10:30:00", "dateupload": "1655286600"},
		{"id": "3", "secret": "ccc", "server": "1", "title": "new", "datetaken": "2024-06-15 10:30:00", "dateupload": "1718441400"},
	}

	var mock *mockserver.FlickrMock
	mock = mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondJSON(200, flickrPhotosResponse(photos, 1, 1, 3))).
		OnGetSizes(func(w http.ResponseWriter, r *http.Request) {
			photoID := r.URL.Query().Get("photo_id")
			mockserver.RespondJSON(200, flickrSizesResponse(
				mock.Server.URL+"/download/"+photoID+".jpg",
			))(w, r)
		}).
		OnDownload(mockserver.RespondBytes(200, testImageData)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.APIURL)

	err := executeCmd(t, "flickr", "download", "--date-range", "2021-01-01:2023-12-31", outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only photo 2 should have been downloaded
	if _, err := os.Stat(filepath.Join(outputDir, "2_bbb.jpg")); err != nil {
		t.Error("photo 2 should have been downloaded (within date range)")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "1_aaa.jpg")); err == nil {
		t.Error("photo 1 should NOT have been downloaded (before date range)")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "3_ccc.jpg")); err == nil {
		t.Error("photo 3 should NOT have been downloaded (after date range)")
	}

	// Transfer log should only have photo 2's ID
	logLines := readLines(t, filepath.Join(outputDir, "transfer.log"))
	if len(logLines) != 1 {
		t.Errorf("transfer log has %d entries, want 1", len(logLines))
	}
	if len(logLines) > 0 && logLines[0] != "2" {
		t.Errorf("transfer log entry = %q, want %q", logLines[0], "2")
	}
}

func TestFlickrDownload_DateRangeIncludesNoDate(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	photos := []map[string]string{
		{"id": "1", "secret": "aaa", "server": "1", "title": "dated", "datetaken": "2022-06-15 10:30:00", "dateupload": "1655286600"},
		{"id": "2", "secret": "bbb", "server": "1", "title": "no-date"},
		{"id": "3", "secret": "ccc", "server": "1", "title": "out-of-range", "datetaken": "2019-01-01 00:00:00", "dateupload": "1546300800"},
	}

	var mock *mockserver.FlickrMock
	mock = mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondJSON(200, flickrPhotosResponse(photos, 1, 1, 3))).
		OnGetSizes(func(w http.ResponseWriter, r *http.Request) {
			photoID := r.URL.Query().Get("photo_id")
			mockserver.RespondJSON(200, flickrSizesResponse(
				mock.Server.URL+"/download/"+photoID+".jpg",
			))(w, r)
		}).
		OnDownload(mockserver.RespondBytes(200, testImageData)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.APIURL)

	err := executeCmd(t, "flickr", "download", "--date-range", "2021-01-01:2023-12-31", outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Photo 1 (in range) and photo 2 (no date, included) should be downloaded
	if _, err := os.Stat(filepath.Join(outputDir, "1_aaa.jpg")); err != nil {
		t.Error("photo 1 should have been downloaded (within date range)")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "2_bbb.jpg")); err != nil {
		t.Error("photo 2 should have been downloaded (no date = included)")
	}
	// Photo 3 (out of range) should NOT be downloaded
	if _, err := os.Stat(filepath.Join(outputDir, "3_ccc.jpg")); err == nil {
		t.Error("photo 3 should NOT have been downloaded (before date range)")
	}
}

func TestFlickrDownload_VideoMetadata(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	mp4Data := buildMinimalMP4()

	photos := []map[string]any{
		{
			"id": "1", "secret": "aaa", "server": "1",
			"title":       "Beach Video",
			"datetaken":   "2020-06-15 14:30:00",
			"dateupload":  "1592234567",
			"description": map[string]string{"_content": "A day at the beach"},
			"tags":        "beach ocean",
		},
	}

	var mock *mockserver.FlickrMock
	mock = mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondJSON(200, map[string]any{
			"photos": map[string]any{
				"page":  1,
				"pages": 1,
				"total": 1,
				"photo": photos,
			},
			"stat": "ok",
		})).
		OnGetSizes(func(w http.ResponseWriter, r *http.Request) {
			mockserver.RespondJSON(200, flickrMultiSizesResponse([]map[string]string{
				{"label": "Video Original", "source": mock.Server.URL + "/download/video.mp4"},
			}))(w, r)
		}).
		OnDownload(mockserver.RespondBytes(200, mp4Data)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.APIURL)

	err := executeCmd(t, "flickr", "download", outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	filePath := filepath.Join(outputDir, "1_aaa.mp4")

	// Verify file exists
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("video file should have been downloaded: %v", err)
	}

	// Verify XMP metadata was embedded in the MP4
	xmp := readXMPFromMP4(t, filePath)
	if xmp == "" {
		t.Fatal("no XMP metadata found in downloaded MP4")
	}
	if !strings.Contains(xmp, "Beach Video") {
		t.Errorf("XMP should contain title, got: %s", xmp)
	}
	if !strings.Contains(xmp, "A day at the beach") {
		t.Errorf("XMP should contain description, got: %s", xmp)
	}
	if !strings.Contains(xmp, "<rdf:li>beach</rdf:li>") {
		t.Errorf("XMP should contain tags, got: %s", xmp)
	}
	if !strings.Contains(xmp, "<xmp:CreateDate>2020-06-15T14:30:00Z</xmp:CreateDate>") {
		t.Errorf("XMP should contain CreateDate from date_taken, got: %s", xmp)
	}

	// Verify file system timestamp was set
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	expectedTime := time.Date(2020, 6, 15, 14, 30, 0, 0, time.UTC)
	modTime := info.ModTime().UTC()
	if !modTime.Equal(expectedTime) {
		t.Errorf("file mod time = %v, want %v", modTime, expectedTime)
	}
}

func TestFlickrUpload_ConsecutiveFailureAbort(t *testing.T) {
	inputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	// Create more than 10 files to trigger the abort threshold
	for i := range 12 {
		_ = os.WriteFile(filepath.Join(inputDir, fmt.Sprintf("photo%02d.jpg", i)), testImageData, 0644)
	}

	mock := mockserver.NewFlickr(t).
		OnUpload(mockserver.RespondStatus(500)). // All uploads fail
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_UPLOAD_URL", mock.UploadURL)

	err := executeCmd(t, "flickr", "upload", inputDir)
	if err == nil {
		t.Fatal("expected error from consecutive failure abort")
	}
	if !strings.Contains(err.Error(), "consecutive upload failures") {
		t.Errorf("error should mention consecutive failures, got: %v", err)
	}

	// Verify fewer than 12 upload requests were made (aborted at 10)
	uploadRequests := 0
	for _, req := range mock.Requests() {
		if strings.HasPrefix(req.Path, "/services/upload/") {
			uploadRequests++
		}
	}
	if uploadRequests != 10 {
		t.Errorf("should have aborted at exactly 10 consecutive failures, but made %d requests", uploadRequests)
	}
}

func TestFlickrUpload_PartialFailure(t *testing.T) {
	inputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	// Create files with alphabetical names so processing order is deterministic
	_ = os.WriteFile(filepath.Join(inputDir, "a_good.jpg"), testImageData, 0644)
	_ = os.WriteFile(filepath.Join(inputDir, "b_bad.jpg"), testImageData, 0644)
	_ = os.WriteFile(filepath.Join(inputDir, "c_good.jpg"), testImageData, 0644)

	var mu sync.Mutex
	fileCount := 0
	mock := mockserver.NewFlickr(t).
		OnUpload(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			n := fileCount
			fileCount++
			mu.Unlock()
			if n == 1 {
				// Second file fails
				mockserver.RespondStatus(500)(w, r)
			} else {
				mockserver.RespondStatus(200)(w, r)
			}
		}).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_UPLOAD_URL", mock.UploadURL)

	err := executeCmd(t, "flickr", "upload", inputDir)
	if err != nil {
		t.Fatalf("unexpected error (should continue past single failure): %v", err)
	}

	// Verify all 3 upload requests were made
	uploadRequests := 0
	for _, req := range mock.Requests() {
		if strings.HasPrefix(req.Path, "/services/upload/") {
			uploadRequests++
		}
	}
	if uploadRequests != 3 {
		t.Errorf("got %d upload requests, want 3 (should continue past single failure)", uploadRequests)
	}

	// Verify report shows the failure
	entries, _ := os.ReadDir(inputDir)
	reportFound := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "photo-copy-report-") {
			reportFound = true
			data, _ := os.ReadFile(filepath.Join(inputDir, e.Name()))
			report := string(data)
			if !strings.Contains(report, "succeeded: 2") {
				t.Errorf("report should show 2 succeeded, got: %s", report)
			}
			if !strings.Contains(report, "failed:    1") {
				t.Errorf("report should show 1 failed, got: %s", report)
			}
		}
	}
	if !reportFound {
		t.Error("expected a report file to be written")
	}
}

func TestFlickrUpload_EmptyDirectory(t *testing.T) {
	inputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	// Empty directory (no media files)
	mock := mockserver.NewFlickr(t).
		OnUpload(mockserver.RespondStatus(200)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_UPLOAD_URL", mock.UploadURL)

	err := executeCmd(t, "flickr", "upload", inputDir)
	if err != nil {
		t.Fatalf("unexpected error for empty dir: %v", err)
	}

	// Verify no upload requests
	for _, req := range mock.Requests() {
		if strings.HasPrefix(req.Path, "/services/upload/") {
			t.Error("no upload requests should have been made for empty directory")
		}
	}
}

func TestFlickrDownload_EmptyPhotoList(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	mock := mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondJSON(200, flickrPhotosResponse(nil, 1, 1, 0))).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.APIURL)

	err := executeCmd(t, "flickr", "download", outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only transfer.log and possibly report should exist
	entries, _ := os.ReadDir(outputDir)
	for _, e := range entries {
		if e.Name() != "transfer.log" && !strings.HasPrefix(e.Name(), "photo-copy-report-") {
			t.Errorf("unexpected file in output: %s", e.Name())
		}
	}
}

func TestFlickrDownload_AllPhotosFilteredByDateRange(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	photos := []map[string]string{
		{"id": "1", "secret": "aaa", "server": "1", "title": "old1", "datetaken": "2019-01-01 00:00:00", "dateupload": "1546300800"},
		{"id": "2", "secret": "bbb", "server": "1", "title": "old2", "datetaken": "2018-06-15 00:00:00", "dateupload": "1529020800"},
	}

	mock := mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondJSON(200, flickrPhotosResponse(photos, 1, 1, 2))).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.APIURL)

	err := executeCmd(t, "flickr", "download", "--date-range", "2022-01-01:2023-12-31", outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No photos should have been downloaded
	entries, _ := os.ReadDir(outputDir)
	for _, e := range entries {
		if e.Name() != "transfer.log" && !strings.HasPrefix(e.Name(), "photo-copy-report-") {
			t.Errorf("unexpected file: %s (all photos should have been filtered out)", e.Name())
		}
	}
}

func TestFlickrDownload_PermanentDownloadFailure(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	photos := []map[string]string{
		{"id": "1", "secret": "aaa", "server": "1", "title": "good"},
		{"id": "2", "secret": "bbb", "server": "1", "title": "bad"},
		{"id": "3", "secret": "ccc", "server": "1", "title": "also_good"},
	}

	var mock *mockserver.FlickrMock
	mock = mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondJSON(200, flickrPhotosResponse(photos, 1, 1, 3))).
		OnGetSizes(func(w http.ResponseWriter, r *http.Request) {
			photoID := r.URL.Query().Get("photo_id")
			if photoID == "2" {
				// Return sizes but all URLs will 404
				mockserver.RespondJSON(200, flickrMultiSizesResponse([]map[string]string{
					{"label": "Original", "source": mock.Server.URL + "/download/missing.jpg"},
				}))(w, r)
			} else {
				mockserver.RespondJSON(200, flickrSizesResponse(
					mock.Server.URL+"/download/"+photoID+".jpg",
				))(w, r)
			}
		}).
		OnDownload(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "missing") {
				w.WriteHeader(404)
				return
			}
			mockserver.RespondBytes(200, testImageData)(w, r)
		}).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.APIURL)

	err := executeCmd(t, "flickr", "download", outputDir)
	if err != nil {
		t.Fatalf("unexpected error (should continue past single failure): %v", err)
	}

	// Photo 1 and 3 should have been downloaded
	if _, err := os.Stat(filepath.Join(outputDir, "1_aaa.jpg")); err != nil {
		t.Error("photo 1 should have been downloaded")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "3_ccc.jpg")); err != nil {
		t.Error("photo 3 should have been downloaded")
	}

	// Transfer log should have 2 successful entries
	logLines := readLines(t, filepath.Join(outputDir, "transfer.log"))
	if len(logLines) != 2 {
		t.Errorf("transfer log has %d entries, want 2", len(logLines))
	}

	// Report should mention the failure
	entries, _ := os.ReadDir(outputDir)
	reportFound := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "photo-copy-report-") {
			reportFound = true
			data, _ := os.ReadFile(filepath.Join(outputDir, e.Name()))
			if !strings.Contains(string(data), "failed:    1") {
				t.Errorf("report should show 1 failure, got: %s", string(data))
			}
		}
	}
	if !reportFound {
		t.Error("expected a report file to be written")
	}
}

// --- Fatal Error Summary Suppression Tests (Flickr) ---

func TestFlickrDownload_FatalError_NoSummary(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	// Server always returns 500 for getPhotos, exhausting retries
	mock := mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondStatus(500)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.APIURL)

	err := executeCmd(t, "flickr", "download", outputDir)
	if err == nil {
		t.Fatal("expected error from exhausted retries")
	}

	// Verify no report file was written (HandleResult was skipped)
	entries, _ := os.ReadDir(outputDir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "photo-copy-report-") {
			t.Errorf("report file %s should not exist on fatal error", e.Name())
		}
	}
}

func TestFlickrUpload_FatalError_NoSummary(t *testing.T) {
	inputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	// Create enough files to trigger the consecutive failure abort (10)
	for i := 0; i < 11; i++ {
		_ = os.WriteFile(filepath.Join(inputDir, fmt.Sprintf("photo%02d.jpg", i)), testImageData, 0644)
	}

	// Upload endpoint always returns 500
	mock := mockserver.NewFlickr(t).
		OnUpload(mockserver.RespondStatus(500)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.APIURL)
	t.Setenv("PHOTO_COPY_FLICKR_UPLOAD_URL", mock.UploadURL)

	err := executeCmd(t, "flickr", "upload", inputDir)
	if err == nil {
		t.Fatal("expected error from consecutive upload failures")
	}

	// Verify no report file was written (HandleResult was skipped)
	entries, _ := os.ReadDir(inputDir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "photo-copy-report-") {
			t.Errorf("report file %s should not exist on fatal error", e.Name())
		}
	}
}
