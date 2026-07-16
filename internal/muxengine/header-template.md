<!-- header-template.md is the default header-pane text template, rendered via
     tokenvocab.Render (internal/tokenvocab) into the always-on operator
     console pane. This leading banner comment is stripped by stencil.Fill
     before parsing, so it documents the template for a human reader only.
     Available top-level tokens (see internal/tokenvocab's registry): {{.repo}}
     (the repo name) and {{.hub}} (the hub's absolute directory path). A
     Config.Header.Template override replaces this whole asset; it is not
     merged with it. -->
hub: {{.hub}}
