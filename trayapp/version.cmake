# Included by the Windows release build; leave non-Windows builds unchanged.
enable_language(RC)
function(oneproxy_version_resource)
    target_sources(oneproxy-tray PRIVATE "${ONEPROXY_VERSION_RESOURCE}")
endfunction()
cmake_language(DEFER CALL oneproxy_version_resource)
