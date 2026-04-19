<!DOCTYPE html>
<html>
<head>
    <title>Zipserver - Books</title>
    <style>
        body { font-family: sans-serif; margin: 40px; line-height: 1.6; }
        table { border-collapse: collapse; width: 100%; }
        th, td { text-align: left; padding: 12px; border-bottom: 1px solid #ddd; }
        tr:hover { background-color: #f5f5f5; }
        a { text-decoration: none; color: #0366d6; font-weight: bold; }
    </style>
</head>
<body>
    <h1>Books</h1>
    <table>
        <tr><th>Book Name</th></tr>
        {{range .}}
        <tr>
            <td>
                <a href="/{{.Name}}/">{{.Name}}</a>
                <span style="margin-left: 20px;"><a href="/{{.Name}}/latest/" style="font-weight: normal; font-size: 0.9em;">[Latest]</a></span>
            </td>
        </tr>
        {{end}}
    </table>
</body>
</html>
