add_executable(oneproxy-platform-test
    platform_test.cpp
    ../platform/platform.h
    ../${ONEPROXY_PLATFORM_SOURCE}
)

target_link_libraries(oneproxy-platform-test PRIVATE Qt6::Test Qt6::Widgets)

add_test(NAME oneproxy-platform-test COMMAND oneproxy-platform-test)
