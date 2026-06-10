---
settings:
  recipes:
    pandoc:
      body-template: "[{output}]({output})"
      params:
        required:
          - from
---
# Multi Output

<?build
recipe: pandoc
from: markdown
inputs:
  - chapters/intro.md
  - chapters/*.md
outputs:
  - book.html
  - book.epub
?>
[book.html](book.html)
[book.epub](book.epub)
<?/build?>
