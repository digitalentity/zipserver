<!DOCTYPE html>
<html>
<head>
    <title>{{.BookName}} versions - Zipserver</title>
    <style>
        body { font-family: sans-serif; margin: 40px; line-height: 1.6; }
        table { border-collapse: collapse; width: 100%; }
        th, td { text-align: left; padding: 12px; border-bottom: 1px solid #ddd; }
        tr:hover { background-color: #f5f5f5; }
        a { text-decoration: none; color: #0366d6; font-weight: bold; }
        .back { margin-bottom: 20px; display: block; }
    </style>
</head>
<body>
    <a href="/" class="back">← Back to Books</a>
    <h1>Versions for {{.BookName}}</h1>
    <div style="margin-bottom: 20px;">
        <a href="/{{.BookName}}/latest/" style="font-weight: normal; font-size: 0.9em;">Permanent link to latest version</a>
    </div>
    <table>
        <tr><th>Version</th><th>Last Modified</th></tr>
        {{range .Versions}}
        <tr>
            <td><a href="/{{$.BookName}}/{{.Name}}/">{{.Name}}</a></td>
            <td>{{.Time}}</td>
        </tr>
        {{end}}
    </table>
</body>
</html>
