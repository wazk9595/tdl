package uploader

import (
	"context"
	stderrors "errors"
	"io"
	"sync"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/go-faster/errors"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/entity"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"

	"github.com/iyear/tdl/core/util/fsutil"
	"github.com/iyear/tdl/core/util/mediautil"
)

// MaxPartSize refer to https://core.telegram.org/api/files#uploading-files
const MaxPartSize = 512 * 1024

type Uploader struct {
	opts Options
}

type Options struct {
	Client   *tg.Client
	Threads  int
	Iter     Iter
	Progress Progress
	Hooks    *Hooks
}

func New(o Options) *Uploader {
	return &Uploader{opts: o}
}

func (u *Uploader) Upload(ctx context.Context, limit int) error {
	wg, wgctx := errgroup.WithContext(ctx)
	wg.SetLimit(limit)
	var failures []error
	var failuresMu sync.Mutex

	for u.opts.Iter.Next(wgctx) {
		elem := u.opts.Iter.Value()

		wg.Go(func() error {
			u.opts.Progress.OnAdd(elem)
			var uploadErr error
			defer func() { u.opts.Progress.OnDone(elem, uploadErr) }()

			if uploadErr = u.upload(wgctx, elem); uploadErr != nil {
				err := uploadErr
				// canceled by user, so we directly return error to stop all
				if errors.Is(err, context.Canceled) {
					return errors.Wrap(err, "upload")
				}

				failuresMu.Lock()
				failures = append(failures, errors.Wrapf(err, "upload %s", elem.File().Name()))
				failuresMu.Unlock()
			}

			return nil
		})
	}

	// Always wait for in-flight sends before closing the client or returning.
	waitErr := wg.Wait()
	return stderrors.Join(waitErr, u.opts.Iter.Err(), stderrors.Join(failures...))
}

func (u *Uploader) upload(ctx context.Context, elem Elem) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Determine media type before uploading bytes. Bad video metadata must not
	// silently turn a large video into an ordinary document.
	mime, video, asFile, err := inspectMedia(elem)
	if err != nil {
		return err
	}
	var thumbFile tg.InputFileClass
	if thumb, ok := elem.Thumb(); ok {
		thumbFile, err = uploadThumbnail(ctx, u.opts.Client, thumb)
		if err != nil {
			return errors.Wrap(err, "upload thumbnail")
		}
	}

	up := uploader.NewUploader(u.opts.Client).
		WithPartSize(MaxPartSize).
		WithThreads(u.opts.Threads).
		WithProgress(&wrapProcess{
			elem:    elem,
			process: u.opts.Progress,
		})

	f, err := up.Upload(ctx, uploader.NewUpload(elem.File().Name(), elem.File(), elem.File().Size()))
	if err != nil {
		return errors.Wrap(err, "upload file")
	}

	// here convert underlying entities to formatters for message caption
	caption := styling.Custom(func(eb *entity.Builder) error {
		msg, entities := elem.Caption()
		eb.Format(msg, lo.Map(entities, func(item tg.MessageEntityClass, _ int) entity.Formatter {
			return func(_, _ int) tg.MessageEntityClass {
				return item
			}
		})...)
		return nil
	})

	doc := message.UploadedDocument(f, caption).MIME(mime).Filename(elem.File().Name())
	if thumbFile != nil {
		doc.Thumb(thumbFile)
	}

	var media message.MediaOption = doc
	var prepared tg.MessageMediaClass

	switch {
	case asFile:
		media = doc.ForceFile(true)
	case video != nil:
		attr := &tg.DocumentAttributeVideo{Duration: video.Duration.Seconds(), W: video.Width, H: video.Height, SupportsStreaming: video.Streaming}
		codec := video.Codec
		if codec == "hevc" {
			codec = "h265"
		}
		if codec == "h264" || codec == "h265" || codec == "av1" {
			attr.SetVideoCodec(codec)
		}
		// Ask Telegram to materialize the video first, so missing thumbnails or
		// streaming metadata cannot be hidden by a successful send operation.
		uploaded, uploadErr := u.opts.Client.MessagesUploadMedia(ctx, &tg.MessagesUploadMediaRequest{
			Peer: elem.To(), Media: &tg.InputMediaUploadedDocument{
				File: f, Thumb: thumbFile, MimeType: mime, NosoundVideo: !video.HasAudio,
				Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeFilename{FileName: elem.File().Name()}, attr},
			},
		})
		if uploadErr != nil {
			return errors.Wrap(uploadErr, "create video media")
		}
		persisted, uploadErr := validateVideoMedia(uploaded, video.Streaming, thumbFile != nil)
		if uploadErr != nil {
			return uploadErr
		}
		prepared = uploaded
		media = message.Media(&tg.InputMediaDocument{ID: persisted.AsInput()}, caption)
	case mediautil.IsImage(mime) && elem.AsPhoto():
		// webp should be uploaded as document
		if mime == "image/webp" {
			break
		}
		// upload as photo
		media = message.UploadedPhoto(f, caption)
	case mediautil.IsAudio(mime):
		media = doc.Audio().Title(fsutil.GetNameWithoutExt(elem.File().Name()))
	}

	sender := taskSender(u.opts.Client, u.opts.Hooks).WithUploader(up)
	if hooks := u.opts.Hooks; hooks != nil {
		if prepared == nil {
			prepared, err = sender.To(elem.To()).UploadMedia(ctx, media)
			if err != nil {
				return errors.Wrap(err, "prepare task media")
			}
		}
		stored, err := storedMedia(prepared)
		if err != nil {
			return err
		}
		media = message.Media(stored, caption)
		if hooks.BeforeSend != nil {
			if err := hooks.BeforeSend(ctx, prepared); err != nil {
				return err
			}
		}
	}
	updates, err := sender.
		WithUploader(up).
		To(elem.To()).
		Reply(elem.Thread()).
		Media(ctx, media)
	if err != nil {
		return errors.Wrap(err, "send message")
	}
	if hooks := u.opts.Hooks; hooks != nil && hooks.Sent != nil {
		id, err := sentID(updates, hooks.RandomID)
		if err != nil {
			return err
		}
		if err := hooks.Sent(ctx, id); err != nil {
			return err
		}
	}

	return nil
}

func uploadThumbnail(ctx context.Context, client uploader.Client, thumb File) (tg.InputFileClass, error) {
	if thumb.Size() <= 0 || thumb.Size() > 200*1024 {
		return nil, errors.New("thumbnail must have a known size between 1 and 200 KB")
	}
	if _, err := thumb.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	// FromReader uses unknown-size streaming and returns InputFileBig, which
	// Telegram cannot use as a thumbnail. Always supply the actual small size.
	return uploader.NewUploader(client).Upload(ctx, uploader.NewUpload(thumb.Name(), thumb, thumb.Size()))
}

func validateVideoMedia(media tg.MessageMediaClass, streaming, thumbnail bool) (*tg.Document, error) {
	m, ok := media.(*tg.MessageMediaDocument)
	if !ok || !m.Video {
		return nil, errors.New("Telegram did not recognize the upload as video")
	}
	doc, ok := m.Document.(*tg.Document)
	if !ok {
		return nil, errors.New("Telegram returned no video document")
	}
	if thumbnail && len(doc.Thumbs) == 0 {
		return nil, errors.New("Telegram did not retain the video thumbnail")
	}
	for _, attribute := range doc.Attributes {
		if attr, ok := attribute.(*tg.DocumentAttributeVideo); ok {
			if streaming && !attr.SupportsStreaming {
				return nil, errors.New("Telegram did not retain streaming support")
			}
			return doc, nil
		}
	}
	return nil, errors.New("Telegram returned no video metadata")
}

func inspectMedia(elem Elem) (string, *mediautil.VideoInfo, bool, error) {
	file := elem.File()
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", nil, false, err
	}
	mime, err := mimetype.DetectReader(file)
	if err != nil {
		return "", nil, false, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", nil, false, err
	}
	if e, ok := elem.(interface{ AsFile() bool }); ok && e.AsFile() && mediautil.IsVideoFile(file.Name(), mime.String()) {
		return mime.String(), nil, true, nil
	}
	if e, ok := elem.(interface{ VideoInfo() *mediautil.VideoInfo }); ok && e.VideoInfo() != nil {
		return mime.String(), e.VideoInfo(), false, nil
	}
	if !mediautil.IsVideo(mime.String()) {
		return mime.String(), nil, false, nil
	}
	duration, w, h, err := mediautil.GetMP4Info(file)
	if err != nil {
		return "", nil, false, errors.Wrap(err, "video metadata")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", nil, false, err
	}
	return mime.String(), &mediautil.VideoInfo{Duration: time.Duration(duration) * time.Second, Width: w, Height: h, HasAudio: true}, false, nil
}
