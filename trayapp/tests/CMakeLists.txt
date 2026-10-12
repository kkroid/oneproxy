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

add_executable(oneproxy-subscription-test subscription_status_test.cpp)
target_link_libraries(oneproxy-subscription-test PRIVATE Qt6::Test)
add_test(NAME oneproxy-subscription-test COMMAND oneproxy-subscription-test)

# Compile the real tray startup/stop methods with deterministic DLL-boundary
# doubles; keep test access changes in the build tree, not production code.
set_property(DIRECTORY APPEND PROPERTY CMAKE_CONFIGURE_DEPENDS
    "${CMAKE_CURRENT_SOURCE_DIR}/../main.cpp" "${CMAKE_CURRENT_SOURCE_DIR}/startup_stop_test.cpp")
file(READ "${CMAKE_CURRENT_SOURCE_DIR}/../main.cpp" startup_tray_source)
string(REPLACE "private:" "public:" startup_tray_source "${startup_tray_source}")
string(REPLACE "int main(" "int trayApplicationMain(" startup_tray_source "${startup_tray_source}")
file(READ "${CMAKE_CURRENT_SOURCE_DIR}/startup_stop_test.cpp" startup_test_source)
file(MAKE_DIRECTORY "${CMAKE_CURRENT_BINARY_DIR}/startup")
file(WRITE "${CMAKE_CURRENT_BINARY_DIR}/startup/main.cpp" "${startup_tray_source}\n${startup_test_source}")
add_executable(oneproxy-startup-stop-test "${CMAKE_CURRENT_BINARY_DIR}/startup/main.cpp"
    ../instance_guard.cpp ../${ONEPROXY_PLATFORM_SOURCE})
target_include_directories(oneproxy-startup-stop-test PRIVATE ..)
target_link_libraries(oneproxy-startup-stop-test PRIVATE Qt6::Widgets)
add_test(NAME oneproxy-startup-stop-test COMMAND oneproxy-startup-stop-test)
set_tests_properties(oneproxy-startup-stop-test PROPERTIES ENVIRONMENT "QT_QPA_PLATFORM=offscreen" TIMEOUT 10)
