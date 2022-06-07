#!/bin/sh

# quick regression test

URLS='https://www.publicdomainpictures.net/pictures/10000/velka/full-moon-11287159978joRe.jpg https://www.publicdomainpictures.net/pictures/10000/velka/1-12106831258yNO.jpg'
EXTENSIONS='jpg png webp heif'

PRODUCTION_SERVER=https://thumbnail.smartnews.com
STAGING_SERVER=http://thumbnail-proxy-stg-1362048176.ap-northeast-1.elb.amazonaws.com
LOCAL_SERVER=http://localhost:8000

mkdir images

for url in $URLS
do
  for extension in $EXTENSIONS
  do
    filename=${url##*/}
    filename=images/${filename%.*}
    stg_filename=$filename-stg.$extension
    prd_filename=$filename-prd.$extension
    local_filename=$filename-local.$extension
    params="?url=$url&q=50&fo=$extension"
    curl -s "$PRODUCTION_SERVER/$params" --output $prd_filename
#    curl -s "$STAGING_SERVER/$params" --output $stg_filename
    curl -s "$LOCAL_SERVER/$params" --output $local_filename
    cmp $prd_filename $local_filename
    if [ $? -eq 0 ]; then
      echo "$filename.$extension: OK"
      rm $prd_filename $local_filename
    else
      echo "$filename.$extension: FAILED"
    fi
  done
done
