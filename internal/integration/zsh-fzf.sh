# dirstat wrapper for zsh with fzf integration
dirstat() {
  emulate -L zsh
  set -o pipefail

  local integration_mode=0
  local -a passthru=()

  for arg in "$@"; do
    if [[ "${arg}" == "-I" ]]; then
      integration_mode=1
      continue
    fi
    passthru+=("${arg}")
  done

  if (( integration_mode )); then
    local output rendered rc
    output=$(command dirstat "${passthru[@]}" 2>&1)
    rc=$?

    if (( rc != 0 )); then
      [[ -n "${output}" ]] && print -u2 -- "${output}"
      return ${rc}
    fi

    rendered=$(
      local header_pattern preview_cmd
      # Detect if we're in directory mode or file mode
      if grep -q "^Top directories:" <<< "${output}"; then
        header_pattern="Top directories:"
        # In directory mode: show filenames
        preview_cmd="command ls -Alh --group-directories-first --time-style=long-iso --color=always -- {1}"
      else
        header_pattern="Top files:"
        # In files mode: keep previous behavior (strip last field)
        preview_cmd="command ls -Alh --group-directories-first --time-style=long-iso --color=always -- {1} | awk '{NF--; print}'"
      fi

      sed -n "/^${header_pattern}/,/^Stats/p" <<< "${output}" |
      grep '^  [0-9]\+)' |
      sed -E "s/^[[:space:]]*[0-9]+\)[[:space:]]+//" |
      sed -E "s/'([^']*)'/\1\t/" |
      awk '{a[NR]=$0} END {for(i=NR;i>0;i--) print a[i]}' |
      SHELL={{ .ZSH }} fzf --wrap --multi \
          --delimiter=$'\t' \
          --with-nth=1 \
          --preview-window=wrap \
          --preview="$preview_cmd" |
      while IFS= read -r file; do
        local file_path="${file%%$'\t'*}"
        file_path="${file_path//\'/\\\'}"
        printf "rm -rf -- '%s'\n" "${file_path}"
      done
    )


    [[ -z "${rendered}" ]] && return 0

    print -z "${rendered%$'\n'}"

    return $?
  fi

  command dirstat "$@"
}
