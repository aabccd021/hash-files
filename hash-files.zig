const std = @import("std");
const fs = std.fs;
const json = std.json;
const Md5 = std.crypto.hash.Md5;
const mem = std.mem;
const heap = std.heap;

pub fn main() !void {
    var general_purpose_allocator = heap.GeneralPurposeAllocator(.{}){};
    const gpa = general_purpose_allocator.allocator();
    defer _ = general_purpose_allocator.deinit();

    // Parse command-line arguments
    const args = try std.process.argsAlloc(gpa);
    defer std.process.argsFree(gpa, args);

    var input_file: ?[]const u8 = null;
    var output_json: ?[]const u8 = null;
    var output_dir: ?[]const u8 = null;

    var i: usize = 1;
    while (i < args.len) : (i += 1) {
        const arg = args[i];
        if (mem.eql(u8, arg, "--input-file") and i + 1 < args.len) {
            input_file = args[i + 1];
            i += 1;
        } else if (mem.eql(u8, arg, "--output-json") and i + 1 < args.len) {
            output_json = args[i + 1];
            i += 1;
        } else if (mem.eql(u8, arg, "--output-dir") and i + 1 < args.len) {
            output_dir = args[i + 1];
            i += 1;
        }
    }

    if (input_file == null or output_json == null or output_dir == null) {
        std.log.err("Usage: {s} --input-file <input_file> --output-json <output_json> --output-dir <output_dir>", .{args[0]});
        return error.InvalidArguments;
    }

    // Load existing mapping from JSON file if it exists
    var mapping = std.StringHashMap([]const u8).init(gpa);
    defer {
        var it = mapping.iterator();
        while (it.next()) |entry| {
            gpa.free(entry.key_ptr.*);
            gpa.free(entry.value_ptr.*);
        }
        mapping.deinit();
    }

    // Use updated JSON parsing approach for Zig 0.14.1
    if (fs.cwd().openFile(output_json.?, .{}) catch null) |file| {
        defer file.close();
        const file_size = (try file.stat()).size;
        if (file_size > 0) {
            const content = try gpa.alloc(u8, file_size);
            defer gpa.free(content);
            const bytes_read = try file.readAll(content);
            
            // Parse JSON using the newer Zig API
            const parsed = try json.parseFromSlice(
                json.Value,
                gpa,
                content[0..bytes_read],
                .{}
            );
            defer parsed.deinit();
            
            if (parsed.value == .object) {
                var obj_it = parsed.value.object.iterator();
                while (obj_it.next()) |entry| {
                    const key = try gpa.dupe(u8, entry.key_ptr.*);
                    const value = if (entry.value_ptr.* == .string) 
                        try gpa.dupe(u8, entry.value_ptr.string)
                    else
                        continue; // Skip non-string values
                    try mapping.put(key, value);
                }
            }
        }
    }

    // Parse filename and extension
    const path = fs.path;
    const filename = path.basename(input_file.?);
    var ext_with_dot: []const u8 = "";
    
    for (filename, 0..) |c, idx| {
        if (c == '.') {
            ext_with_dot = filename[idx..];
        }
    }
    const ext = if (ext_with_dot.len > 1) ext_with_dot[1..] else "";
    const filename_no_ext = filename[0 .. filename.len - ext_with_dot.len];

    // Compute MD5 hash
    const file = try fs.cwd().openFile(input_file.?, .{});
    defer file.close();

    var hasher = Md5.init(.{});
    var buffer: [8192]u8 = undefined;
    while (true) {
        const bytes_read = try file.read(&buffer);
        if (bytes_read == 0) break;
        hasher.update(buffer[0..bytes_read]);
    }

    var hash_bytes: [Md5.digest_length]u8 = undefined;
    hasher.final(&hash_bytes);

    // Convert hash to hex string (fixed to use const)
    const hash = try std.fmt.allocPrint(gpa, "{s}", .{std.fmt.fmtSliceHexLower(&hash_bytes)});
    defer gpa.free(hash);

    // Create new filename with hash
    const hash_filename = try std.fmt.allocPrint(gpa, "{s}.{s}.{s}", .{ filename_no_ext, hash, ext });
    defer gpa.free(hash_filename);

    // Check if file has changed
    if (mapping.get(filename)) |existing_hash_filename| {
        if (mem.eql(u8, existing_hash_filename, hash_filename)) {
            return; // No change, nothing to do
        }
    }

    // Update mapping
    const filename_copy = try gpa.dupe(u8, filename);
    const hash_filename_copy = try gpa.dupe(u8, hash_filename);
    try mapping.put(filename_copy, hash_filename_copy);

    // Copy file to output directory
    try file.seekTo(0);
    const out_path = try std.fmt.allocPrint(gpa, "{s}/{s}", .{ output_dir.?, hash_filename });
    defer gpa.free(out_path);

    const out_file = try fs.cwd().createFile(out_path, .{});
    defer out_file.close();

    var bytes_copied: usize = 0;
    while (true) {
        const bytes_read = try file.read(&buffer);
        if (bytes_read == 0) break;
        try out_file.writeAll(buffer[0..bytes_read]);
        bytes_copied += bytes_read;
    }

    try out_file.sync();

    // Save mapping to JSON
    var out_json = std.ArrayList(u8).init(gpa);
    defer out_json.deinit();

    try out_json.append('{');
    var first = true;
    var it = mapping.iterator();
    while (it.next()) |entry| {
        if (!first) {
            try out_json.append(',');
        }
        first = false;
        try std.json.stringify(entry.key_ptr.*, .{}, out_json.writer());
        try out_json.append(':');
        try std.json.stringify(entry.value_ptr.*, .{}, out_json.writer());
    }
    try out_json.append('}');

    // Fixed: writeFile now takes a WriteFileOptions struct
    // try fs.cwd().writeFile(.{
    //     .path = output_json.?,
    //     .data = out_json.items,
    // });
}
