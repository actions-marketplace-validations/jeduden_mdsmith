# Build directive demo

The `render` recipe is a user-defined shell script. `mdsmith fix`
runs it and keeps `artifact.txt` in sync with `source.txt`.

<?build
recipe: render
inputs:
  - source.txt
outputs:
  - artifact.txt
?>
[artifact.txt](artifact.txt)
<?/build?>
