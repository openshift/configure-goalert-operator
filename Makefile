FIPS_ENABLED=true

# saas files for CGO do not and will not exist in commercial
# we need to skip saas checks during csv-generate.sh
export SKIP_SAAS_FILE_CHECKS=y

include boilerplate/generated-includes.mk


.PHONY: boilerplate-update
boilerplate-update:
	@boilerplate/update
