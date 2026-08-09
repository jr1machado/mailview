package main

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/knadh/listmonk/internal/mailview/tenant"
	internalmedia "github.com/knadh/listmonk/internal/media"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

const (
	thumbPrefix   = "thumb_"
	thumbnailSize = 250
)

var (
	vectorExts = []string{"svg"}
	imageExts  = []string{"gif", "png", "jpg", "jpeg"}
)

// UploadMedia handles media file uploads.
func (a *App) UploadMedia(c echo.Context) error {
	file, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("media.invalidFile", "error", err.Error()))
	}

	// Read the file from the HTTP form.
	src, err := file.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			a.i18n.Ts("media.errorReadingFile", "error", err.Error()))
	}
	defer src.Close()

	var (
		// Naive check for content type and extension.
		ext         = strings.TrimPrefix(strings.ToLower(filepath.Ext(file.Filename)), ".")
		contentType = file.Header.Get("Content-Type")
	)

	// Validate file extension.
	if !inArray("*", a.cfg.MediaUpload.Extensions) {
		if ok := inArray(ext, a.cfg.MediaUpload.Extensions); !ok {
			return echo.NewHTTPError(http.StatusBadRequest,
				a.i18n.Ts("media.unsupportedFileType", "type", ext))
		}
	}

	// Sanitize the filename.
	fName := makeFilename(file.Filename)
	if scoped, ok := c.Get("mailview_tenant_context").(context.Context); ok {
		if scope, ok := tenant.FromContext(scoped); ok {
			fName = scope.TenantID.String() + "/" + fName
		}
	}

	// If the filename already exists in the DB, make it unique by adding a random suffix.
	if _, err := a.mailviewCore(c).GetMedia(0, "", fName, a.media); err == nil {
		suffix, err := generateRandomString(6)
		if err != nil {
			a.log.Printf("error generating random string: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError, a.i18n.T("globals.messages.internalError"))
		}

		fName = appendSuffixToFilename(fName, suffix)
	}

	// Upload the file to the media store.
	fName, err = a.media.Put(fName, contentType, src)
	if err != nil {
		a.log.Printf("error uploading file: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			a.i18n.Ts("media.errorUploading", "error", err.Error()))
	}

	// This keeps track of whether the file has to be deleted from the DB and the store
	// if any of the subsequent steps fail.
	var (
		cleanUp    = false
		thumbfName = ""
	)
	defer func() {
		if cleanUp {
			a.media.Delete(fName)

			if thumbfName != "" {
				a.media.Delete(thumbfName)
			}
		}
	}()

	// Thumbnail width and height.
	var width, height int

	// Create thumbnail from file for non-vector formats.
	isImage := inArray(ext, imageExts)
	if isImage {
		thumbFile, wi, he, err := processImage(file)
		if err != nil {
			cleanUp = true
			a.log.Printf("error resizing image: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				a.i18n.Ts("media.errorResizing", "error", err.Error()))
		}
		width = wi
		height = he

		// Upload thumbnail.
		thumbName := filepath.Join(filepath.Dir(fName), thumbPrefix+filepath.Base(fName))
		tf, err := a.media.Put(thumbName, contentType, thumbFile)
		if err != nil {
			cleanUp = true
			a.log.Printf("error saving thumbnail: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				a.i18n.Ts("media.errorSavingThumbnail", "error", err.Error()))
		}
		thumbfName = tf
	}
	if inArray(ext, vectorExts) {
		thumbfName = fName
	}

	// Images have metadata.
	meta := models.JSON{}
	if isImage {
		meta = models.JSON{
			"width":  width,
			"height": height,
		}
	}

	// Insert the media into the DB.
	m, err := a.mailviewCore(c).InsertMedia(fName, thumbfName, contentType, meta, a.cfg.MediaUpload.Provider, a.media)
	if err != nil {
		cleanUp = true
		return err
	}

	a.scopeTenantMediaURLs(c, &m)
	return c.JSON(http.StatusOK, okResp{m})
}

// GetAllMedia handles retrieval of uploaded media.
func (a *App) GetAllMedia(c echo.Context) error {
	var (
		query = c.FormValue("query")

		pg = a.pg.NewFromURL(c.Request().URL.Query())
	)
	// Fetch the media items from the DB.
	res, total, err := a.mailviewCore(c).QueryMedia(a.cfg.MediaUpload.Provider, a.media, query, pg.Offset, pg.Limit)
	if err != nil {
		return err
	}
	for i := range res {
		a.scopeTenantMediaURLs(c, &res[i])
	}

	out := models.PageResults{
		Results: res,
		Total:   total,
		Page:    pg.Page,
		PerPage: pg.PerPage,
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// GetMedia handles retrieval of a media item by ID.
func (a *App) GetMedia(c echo.Context) error {
	// Fetch the media item from the DB.
	id := getID(c)
	out, err := a.mailviewCore(c).GetMedia(id, "", "", a.media)
	if err != nil {
		return err
	}
	a.scopeTenantMediaURLs(c, &out)

	return c.JSON(http.StatusOK, okResp{out})
}

// DeleteMedia handles deletion of uploaded media.
func (a *App) DeleteMedia(c echo.Context) error {
	id := getID(c)
	mediaItem, err := a.mailviewCore(c).GetMedia(id, "", "", a.media)
	if err != nil {
		return err
	}

	// Delete the media from the DB. The query returns the filename.
	fname, err := a.mailviewCore(c).DeleteMedia(id)
	if err != nil {
		return err
	}

	// Delete the files from the media store.
	if err := a.media.Delete(fname); err != nil {
		a.log.Printf("error deleting media file %s: %v", fname, err)
	}
	if mediaItem.Thumb != "" && mediaItem.Thumb != fname {
		if err := a.media.Delete(mediaItem.Thumb); err != nil {
			a.log.Printf("error deleting media thumbnail %s: %v", mediaItem.Thumb, err)
		}
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// ServeMedia serves filesystem or S3 media through the application when the
// configured public URL is relative. On tenant hosts, the object path must
// contain exactly the tenant prefix established by UploadMedia.
func (a *App) ServeMedia(c echo.Context) error {
	key := strings.TrimPrefix(c.Param("*"), "/")
	if key == "" {
		key = strings.TrimPrefix(c.Param("filepath"), "/")
	}
	if key == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing media file path")
	}
	key, err := validateTenantMediaKey(c, key)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "media not found")
	}

	b, err := a.media.GetBlob(key)
	if err != nil {
		a.log.Printf("error fetching media %s: %v", key, err)
		return echo.NewHTTPError(http.StatusNotFound, "media not found")
	}

	return c.Stream(http.StatusOK, http.DetectContentType(b), bytes.NewReader(b))
}

func validateTenantMediaKey(c echo.Context, rawKey string) (string, error) {
	scoped, ok := c.Get("mailview_tenant_context").(context.Context)
	if !ok {
		return internalmedia.ValidateTenantPath(rawKey, "")
	}
	scope, ok := tenant.FromContext(scoped)
	if !ok {
		return "", echo.NewHTTPError(http.StatusForbidden, "tenant context missing")
	}
	return internalmedia.ValidateTenantPath(rawKey, scope.TenantID.String())
}

// scopeTenantMediaURLs turns URLs hosted by the application's configured
// root into relative URLs on tenant requests. The browser then keeps the
// verified tenant hostname instead of crossing over to the global host.
// External S3/CDN URLs and private presigned URLs remain untouched.
func (a *App) scopeTenantMediaURLs(c echo.Context, item *internalmedia.Media) {
	if c.Get("mailview_tenant_context") == nil || item == nil || a.urlCfg == nil {
		return
	}
	root, err := url.Parse(a.urlCfg.RootURL)
	if err != nil || root.Host == "" {
		return
	}
	relative := func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" || !strings.EqualFold(parsed.Host, root.Host) {
			return raw
		}
		return parsed.RequestURI()
	}
	item.URL = relative(item.URL)
	if item.ThumbURL.Valid {
		item.ThumbURL.String = relative(item.ThumbURL.String)
	}
}

// processImage reads the image file and returns thumbnail bytes and
// the original image's width, and height.
func processImage(file *multipart.FileHeader) (*bytes.Reader, int, int, error) {
	src, err := file.Open()
	if err != nil {
		return nil, 0, 0, err
	}
	defer src.Close()

	img, err := imaging.Decode(src)
	if err != nil {
		return nil, 0, 0, err
	}

	// Encode the image into a byte slice as PNG.
	var (
		thumb = imaging.Resize(img, thumbnailSize, 0, imaging.Lanczos)
		out   bytes.Buffer
	)
	if err := imaging.Encode(&out, thumb, imaging.PNG); err != nil {
		return nil, 0, 0, err
	}

	b := img.Bounds().Max
	return bytes.NewReader(out.Bytes()), b.X, b.Y, nil
}
