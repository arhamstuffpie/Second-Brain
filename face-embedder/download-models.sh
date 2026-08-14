#!/bin/sh
set -eu

model_dir="${1:-$(dirname "$0")/models}"
mkdir -p "$model_dir"

curl -fL "https://huggingface.co/opencv/face_detection_yunet/resolve/main/face_detection_yunet_2023mar.onnx" \
  -o "$model_dir/face_detection_yunet_2023mar.onnx"
curl -fL "https://huggingface.co/opencv/face_recognition_sface/resolve/main/face_recognition_sface_2021dec.onnx" \
  -o "$model_dir/face_recognition_sface_2021dec.onnx"

printf '%s  %s\n' \
  8f2383e4dd3cfbb4553ea8718107fc0423210dc964f9f4280604804ed2552fa4 \
  "$model_dir/face_detection_yunet_2023mar.onnx" \
  0ba9fbfa01b5270c96627c4ef784da859931e02f04419c829e83484087c34e79 \
  "$model_dir/face_recognition_sface_2021dec.onnx" | shasum -a 256 -c -
