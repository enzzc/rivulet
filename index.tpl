<!DOCTYPE html>
<html lang="en">
  <head>
    <!-- Required meta tags -->
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1, shrink-to-fit=no">
    <title>Rivulet</title>
    <style>
        body {
            margin: auto;
            padding: 1rem;
            max-width: 600px;
            font-family: Roboto, -apple-system, 'Helvetica Neue', 'Droid Sans', 'Noto Sans', 'Segoe UI', SegoeUI, sans-serif;
        }
        .item { margin-top: 2rem; margin-bottom: 2rem; }
        .item-meta { font-size: 0.8rem; }
        h1.main-title { font-size: inherit; }
        span.domain {
            display: block;
            font-family: monospace;
            font-size: 0.9em;
            margin-top: 0.9em;
        }
        @media (prefers-color-scheme: dark) {
            body {
                background-color: #121212;
                color: #ececec;
            }
            a { color: #7896e3; }
            a:visited { color: #bb86fc; }
        }
    </style>
  </head>
  <body>

  <div class="container" id="">
    <header class="row" id="header">
        <h1 class="main-title">
            {{ if .Domain }}
                <a href="/">Rivulet</a>
                <span class="domain">{{ .Domain }}</span>
            {{ else }}
                Rivulet
            {{ end }}
        </h1>
    </header>

    <main class="container" id="feed">
        <div class="row">
            {{ range .Items }}
            <div class="col-md-8 offset-md-2">
                <div class="row post-item">
                    <div class="item">
                        <p class="item-main"><a href="{{ .Link }}">{{ .Title }}</a></p>
                        <div class="item-meta">
                            <time datetime="{{ .ShortDateDisplay }}"><i>{{ .ShortDateDisplay }}</i></time> &ndash;
                            <a class="domain" href="?domain=https://{{ .Domain }}">
                                {{ .ShortDomainDisplay }}
                            </a>
                        </div>
                    </div>
                </div>
            </div>
            {{ end }}
            </div>
    </main>
  </div>
 </div> <!-- end #app .container -->
 <script>
    console.log('ok');
 </script>
</body>
</html>

