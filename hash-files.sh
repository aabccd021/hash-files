input_dir=""
output_json=""
output_dir=""
watch=false

while [ $# -gt 0 ]; do
  case $1 in
  --watch)
    watch=true
    ;;
  --input-dir)
    input_dir=$2
    shift
    ;;
  --output-json)
    output_json=$2
    shift
    ;;
  --output-dir)
    output_dir=$2
    shift
    ;;
  *)
    echo "Unknown argument: $1" >&2
    exit 1
    ;;
  esac
  shift
done

if [ -z "$input_dir" ]; then
  echo "Usage: $0 --input-dir <input_dir> [--output-json <output_json>] [--output-dir <output_dir>]"
  exit 1
fi

while true; do

  if [ "$watch" = true ]; then
    inotifywait --event create "$input_dir" >/dev/null 2>&1
  fi

  if [ ! -f "$output_json" ]; then
    echo "{}" >"$output_json"
  fi

  asset_results=$(ls "$input_dir")

  tmpfile=$(mktemp)
  for filename in $asset_results; do
    filename_no_ext="${filename%.*}"
    ext="${filename##*.}"
    hash=$(md5sum "$input_dir/$filename" | cut -d ' ' -f 1)
    hash_filename="$filename_no_ext.$hash.$ext"

    if [ -n "$output_json" ]; then
      jq ".\"$filename\" = \"$hash_filename\"" "$output_json" >"$tmpfile"

      # avoid unnecessary write by doing it only if output_json is different from tmpfile
      if ! cmp -s "$output_json" "$tmpfile"; then
        truncate -s 0 "$output_json"
        cat "$tmpfile" >>"$output_json"
      fi
    fi

    if [ -n "$output_dir" ]; then
      mv "$input_dir/$filename" "$output_dir/$hash_filename"
    fi
  done

  if [ "$watch" = false ]; then
    break
  fi

done
