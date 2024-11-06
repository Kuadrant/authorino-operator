#!/usr/bin/env bash
# Builds the OLM catalog index manifests and pushes it to quay.io.
# To push to your own registry, override the IMG_REGISTRY_HOST, IMG_REGISTRY_ORG, OPERATOR_NAME, and TAG env vars,
# i.e:
#   IMG_REGISTRY_HOST=quay.io IMG_REGISTRY_ORG=yourusername OPERATOR_NAME=authorino-operator TAG=latest ./script/build_catalog.sh
#
set -e  # Exit on error
IFS=' ' read -r -a tags <<< "$TAG"
architectures=(${ARCHITECTURES})
first_tag="${tags[0]}"

 for arch in "${architectures[@]}"; do
    # Pull the image for all the architecture
    podman pull "${IMG_REGISTRY_HOST}/${IMG_REGISTRY_ORG}/${OPERATOR_NAME}-catalog:${first_tag}-${arch}"
  done

for tag in "${tags[@]}"; do
  echo "Creating manifest for $tag"
  podman manifest create "${IMG_REGISTRY_HOST}/${IMG_REGISTRY_ORG}/${OPERATOR_NAME}-catalog:${tag}"
  
  for arch in "${architectures[@]}"; do
    podman manifest add "${IMG_REGISTRY_HOST}/${IMG_REGISTRY_ORG}/${OPERATOR_NAME}-catalog:${tag}" "docker://${IMG_REGISTRY_HOST}/${IMG_REGISTRY_ORG}/${OPERATOR_NAME}-catalog:${first_tag}-${arch}"
    podman manifest annotate "${IMG_REGISTRY_HOST}/${IMG_REGISTRY_ORG}/${OPERATOR_NAME}-catalog:${tag}" \
      --os "linux" \
      --arch "${arch}" 
  done
  # Push the manifest to the repository
  podman manifest push --all "${IMG_REGISTRY_HOST}/${IMG_REGISTRY_ORG}/${OPERATOR_NAME}-catalog:${tag}" "${IMG_REGISTRY_HOST}/${IMG_REGISTRY_ORG}/${OPERATOR_NAME}-catalog:${tag}"
  # Remove the manifest image after pushing
  podman rmi "${IMG_REGISTRY_HOST}/${IMG_REGISTRY_ORG}/${OPERATOR_NAME}-catalog:${tag}" || true
done

# Clean up images
  for arch in "${architectures[@]}"; do
    echo "Removing image for architecture: ${arch} and tag: ${tag}-${arch}"
    podman rmi "${IMG_REGISTRY_HOST}/${IMG_REGISTRY_ORG}/${OPERATOR_NAME}-catalog:${first_tag}-${arch}" || true
  done