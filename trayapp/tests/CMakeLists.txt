add_executable(oneproxy-platform-test
    platform_test.cpp
    ../platform/platform.h
    ../${ONEPROXY_PLATFORM_SOURCE}
)

target_link_libraries(oneproxy-platform-test PRIVATE Qt6::Test Qt6::Widgets)

add_test(NAME oneproxy-platform-test COMMAND oneproxy-platform-test)

add_executable(oneproxy-instance-helper instance_helper.cpp ../instance_guard.cpp)
target_link_libraries(oneproxy-instance-helper PRIVATE Qt6::Core)
add_executable(oneproxy-instance-test instance_test.cpp ../instance_guard.cpp)
target_link_libraries(oneproxy-instance-test PRIVATE Qt6::Test)
add_dependencies(oneproxy-instance-test oneproxy-instance-helper)
add_test(NAME oneproxy-instance-test COMMAND oneproxy-instance-test)
