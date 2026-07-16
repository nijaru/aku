package bind

import (
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"reflect"
)

// compileForm creates an internalExtractor for the Form section of the request struct.
func compileForm(sectionIdx int, typ reflect.Type) (internalExtractor, []Parameter) {
	if typ.Kind() != reflect.Struct {
		return func(ctx context.Context, r *http.Request, v reflect.Value, cfg *Config) error { return nil }, nil
	}

	type formInfo struct {
		idx      int
		name     string
		isSlice  bool
		required bool
		coercer  Coercer
	}
	var infos []formInfo
	var fileInfos []formInfo
	var params []Parameter

	fileHeaderType := reflect.TypeFor[*multipart.FileHeader]()
	fileHeaderSliceType := reflect.TypeFor[[]*multipart.FileHeader]()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" {
			continue
		}
		tag := field.Tag.Get("form")
		if tag == "" {
			continue
		}

		if field.Type == fileHeaderType || field.Type == fileHeaderSliceType {
			fileInfos = append(
				fileInfos,
				formInfo{
					idx:      i,
					name:     tag,
					isSlice:  field.Type.Kind() == reflect.Slice,
					required: fieldRequired(field),
				},
			)
		} else {
			elemTyp := field.Type
			if field.Type.Kind() == reflect.Slice {
				elemTyp = field.Type.Elem()
			}
			infos = append(infos, formInfo{
				idx:      i,
				name:     tag,
				isSlice:  field.Type.Kind() == reflect.Slice,
				required: fieldRequired(field),
				coercer:  PrecompileCoercer(elemTyp),
			})
		}

		params = append(params, Parameter{
			Name:     tag,
			In:       "form",
			Type:     field.Type,
			Required: fieldRequired(field),
			Validate: field.Tag.Get("validate"),
			Message:  field.Tag.Get("msg"),
			Example:  field.Tag.Get("example"),
		})
	}

	return func(ctx context.Context, r *http.Request, v reflect.Value, cfg *Config) error {
		// Ensure form is parsed.
		// Use dynamic max memory from config.
		if err := r.ParseMultipartForm(cfg.MaxMultipartMemory); err != nil &&
			err != http.ErrNotMultipart {
			return &BindError{Source: "form", Err: err}
		}

		section := v.Field(sectionIdx)

		// Regular form values
		for _, info := range infos {
			if info.isSlice {
				vals := r.PostForm[info.name]
				if len(vals) > 0 {
					f := section.Field(info.idx)
					slice := reflect.MakeSlice(f.Type(), len(vals), len(vals))
					for i, val := range vals {
						if err := info.coercer(val, slice.Index(i)); err != nil {
							return &BindError{Field: info.name, Source: "form", Err: err}
						}
					}
					f.Set(slice)
				} else if info.required {
					return &BindError{Field: info.name, Source: "form", Err: errors.New("is required")}
				}
			} else {
				val := r.PostFormValue(info.name)
				if val != "" {
					if err := info.coercer(val, section.Field(info.idx)); err != nil {
						return &BindError{Field: info.name, Source: "form", Err: err}
					}
				} else if info.required {
					return &BindError{Field: info.name, Source: "form", Err: errors.New("is required")}
				}
			}
		}

		// Multipart files
		if r.MultipartForm != nil {
			for _, info := range fileInfos {
				files := r.MultipartForm.File[info.name]
				if len(files) > 0 {
					f := section.Field(info.idx)
					if info.isSlice {
						slice := reflect.MakeSlice(f.Type(), len(files), len(files))
						for i, fh := range files {
							slice.Index(i).Set(reflect.ValueOf(fh))
						}
						f.Set(slice)
					} else {
						f.Set(reflect.ValueOf(files[0]))
					}
				} else if info.required {
					return &BindError{Field: info.name, Source: "form", Err: errors.New("is required")}
				}
			}
		} else {
			for _, info := range fileInfos {
				if info.required {
					return &BindError{Field: info.name, Source: "form", Err: errors.New("is required")}
				}
			}
		}

		return nil
	}, params
}
